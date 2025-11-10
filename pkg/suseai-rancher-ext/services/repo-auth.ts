export interface RepoAuth { username: string; password: string; }
type SecretRef = string | { name?: string; namespace?: string } | null | undefined;

const WANT_URL = 'oci://dp.apps.rancher.io/charts';
const norm = (u?: string) => (u || '').replace(/\/+$/, '');
const b64 = (s?: string) => { try { return s ? atob(s) : ''; } catch { return ''; } };

// -------------------- secret ref parsing + extraction --------------------

function parseRef(ref: SecretRef): { name: string; namespace: string } | null {
  if (!ref) return null;
  if (typeof ref === 'string') return { name: ref, namespace: 'cattle-system' };
  if (typeof ref === 'object' && ref.name) return { name: ref.name, namespace: ref.namespace || 'cattle-system' };
  return null;
}

export function dockerconfigAuthKeyForHost(host?: string): string {
  const h = (host || '').toLowerCase().trim();
  if (!h || h === 'docker.io' || h === 'index.docker.io' || h.endsWith('.docker.com')) {
    return 'https://index.docker.io/v1/'; // canonical Hub key
  }
  return h;
}

// Generic host parser (supports oci:// and http(s)://)
function parseRegistryHost(url?: string): string {
  if (!url) return '';
  const u = url.trim();
  if (u.startsWith('oci://')) return u.slice('oci://'.length).split('/')[0] || '';
  try { return new URL(u).host || ''; } catch { return u.split('/')[0] || ''; }
}

function extractFromDockerCfg(sec: any): RepoAuth | null {
  const blob = sec?.data?.['.dockerconfigjson'] || sec?.data?.dockerconfigjson;
  if (!blob) return null;
  try {
    const cfg = JSON.parse(b64(blob)); const auths = cfg?.auths || cfg;
    for (const k of Object.keys(auths || {})) {
      const e = auths[k];
      if (e?.auth) {
        const pair = atob(e.auth); const i = pair.indexOf(':');
        if (i > 0) return { username: pair.slice(0, i), password: pair.slice(i + 1) };
      }
      if (e?.username && e?.password) return { username: e.username, password: e.password };
    }
  } catch {}
  return null;
}

function extract(sec: any): RepoAuth | null {
  const data = sec || {};

  // kubernetes.io/basic-auth
  const u = b64(data.username), p = b64(data.password);
  if (u && p) return { username: u, password: p };

  // single "auth" (b64 user:pass)
  if (data.auth) {
    try {
      const pair = atob(b64(data.auth)); const i = pair.indexOf(':');
      if (i > 0) return { username: pair.slice(0, i), password: pair.slice(i + 1) };
    } catch {}
  }

  // token-only
  if (data.token) {
    const t = b64(data.token);
    if (t) return { username: 'token', password: t };
  }

  // accessKey/secretKey
  const ak = b64(data.accessKey), sk = b64(data.secretKey);
  if (ak && sk) return { username: ak, password: sk };

  // dockerconfigjson
  return extractFromDockerCfg(sec);
}

/** Try multiple Rancher paths to get a Secret that actually contains `.data` */
async function fetchSecret(store: any, ns: string, name: string) {
  try {
    const r3 = await store.dispatch('rancher/request', { url: `/v1/secrets/${encodeURIComponent(ns)}/${encodeURIComponent(name)}` });
    const s3 = r3?.data || r3 || {};

    if (Object.keys(s3 || {}).length) return s3;
  } catch {}

  return {};
}

// -------------------- existing export kept (used earlier) --------------------

/** Resolve credentials from the default Application Collection repo. */
export async function getDpRepoAuth(store: any, clusterId: string): Promise<RepoAuth> {
  // Find the repo

  const isLocalCluster = clusterId === 'local';
  const repoUrl = isLocalCluster
    ? `/k8s/clusters/local/apis/catalog.cattle.io/v1/clusterrepos?limit=1000`
    : `/k8s/clusters/${encodeURIComponent(clusterId)}/apis/catalog.cattle.io/v1/clusterrepos?limit=1000`;

  const res = await store.dispatch('rancher/request', { url: repoUrl });
  const items = res?.data?.items || res?.data || res?.items || [];

  const repo = items.find((r: any) => norm(r?.spec?.url) === WANT_URL);
  if (!repo) {
    throw new Error(`Repository "${WANT_URL}" not found on the local cluster. Go to Cluster Management → local → Apps → Repositories and create it (Target: OCI Repository, Auth: BasicAuth).`);
  }

  // Resolve secret reference (supports object/string + legacy fields)
  const ref =
    parseRef(repo?.spec?.clientSecret) ||
    parseRef(repo?.spec?.clientSecretName) ||
    parseRef(repo?.spec?.authSecret) ||
    parseRef(repo?.spec?.authSecretName);

  if (!ref) {
    throw new Error(`Credentials not found. Edit repository "${repo?.metadata?.name}" and set Authentication to BasicAuth (username/password or a Secret).`);
  }

  // Read secret via any working path
  const sec = await fetchSecret(store, ref.namespace, ref.name);
  const auth = extract(sec);

  if (!auth) {
    const keys = Object.keys(sec?.data || {});
    const hint = keys.length ? ` (found keys: ${keys.join(', ')})` : '';
    throw new Error(`Credentials not found. Secret ${ref.namespace}/${ref.name} must contain base64-encoded "username" and "password", a single "auth" (b64 "user:pass"), a "token", "accessKey/secretKey", or a valid ".dockerconfigjson" with auth${hint}.`);
  }

  return auth;
}

// -------------------- NEW: repo → host + secret + creds --------------------

export interface RepoInstallContext {
  /** The registry host derived from spec.url, e.g. "dp.apps.rancher.io" */
  registryHost: string;
  /** The name of the secret referenced by the ClusterRepo (in cattle-system) */
  secretName?: string;
  /** Basic auth (username/password) extracted from that secret */
  auth?: RepoAuth;
}

/** Parse an OCI or HTTPS/HTTP URL into just the registry host */
function registryHostFromUrl(url?: string): string {
  if (!url) return '';
  // Strip scheme (oci://, http://, https://)
  const noScheme = url.replace(/^[a-z]+:\/\//i, '');
  // Host is first path segment
  const host = noScheme.split('/')[0] || '';
  return host;
}

export interface RepoInstallContext {
  registryHost: string;
  secretName?: string;     // the repo's configured secret name (in cattle-system unless overridden)
  auth?: RepoAuth;         // parsed username/password
}

function parseRegistryHostFromOciUrl(url?: string): string {
  // oci://host/path[/...]
  const u = (url || '').trim();
  if (!u.startsWith('oci://')) return '';
  const rest = u.slice('oci://'.length);
  const host = rest.split('/')[0] || '';
  return host;
}

/**
 * Resolve creds + registry host for a specific ClusterRepo (by metadata.name).
 */
export async function getRepoAuthForClusterRepo( store: any, clusterRepoName: string): Promise<RepoInstallContext> {
  if (!clusterRepoName) throw new Error('ClusterRepo name is required');

  let cluster: any = null;
  let clusterId: string | null = null;
  let isLocalCluster = false;

  try {
    const { getClusters } = await import('./rancher-apps');
    const clusters = await getClusters(store);
    console.log(`[SUSE-AI] getRepoAuthForClusterRepo: Found ${clusters.length} clusters`);

    // 🧩 Pick first cluster or prefer 'local' if available
    cluster =
      clusters.find((c: any) => c.id === 'local') ||
      clusters[0];

    if (cluster) {
      clusterId = cluster.id;
      isLocalCluster = cluster.id === 'local';
      console.log(`[SUSE-AI] Matched cluster: ${cluster.id} (${cluster.spec?.displayName || 'no name'})`);
    } else {
      console.warn('[SUSE-AI] No clusters found. Falling back to local.');
      clusterId = 'local';
      isLocalCluster = true;
    }
  } catch (error) {
    console.error('[SUSE-AI] getRepoAuthForClusterRepo: Failed to get clusters:', error);
    // fallback when cluster list fails
    clusterId = 'local';
    isLocalCluster = true;
  }

  // 🧠 Build Rancher API path
  const url = isLocalCluster
    ? `/k8s/clusters/local/apis/catalog.cattle.io/v1/clusterrepos/${encodeURIComponent(clusterRepoName)}`
    : `/k8s/clusters/${encodeURIComponent(clusterId!)}/apis/catalog.cattle.io/v1/clusterrepos/${encodeURIComponent(clusterRepoName)}`;

  const r = await store.dispatch('rancher/request', { url });
  const repo = r?.data ?? r;
  if (!repo?.spec) throw new Error(`ClusterRepo ${clusterRepoName} not found`);

  const registryHost = parseRegistryHost(repo?.spec?.url) || 'docker.io';

  // 🔐 Resolve secret reference (same logic as getDpRepoAuth)
  const ref = repo?.spec?.clientSecret
    ? { name: repo.spec.clientSecret.name, namespace: repo.spec.clientSecret.namespace }
    : null;

  if (!ref) {
    return {
      registryHost,
      secretName: undefined,
      auth: undefined,
    };
  }

  const sec = await fetchSecret(store, ref.namespace, ref.name);
  const auth = extract(sec);
  if (!auth) {
    const keys = Object.keys(sec?.data || {});
    throw new Error(`Credentials not found in ${ref.namespace}/${ref.name}. Found keys: ${keys.join(', ') || 'none'}.`);
  }

  return {
    registryHost,
    secretName: ref.name,
    auth,
  };
}



// Optional alias to satisfy any lingering import:
export const getRepoInstallContext = getRepoAuthForClusterRepo;
