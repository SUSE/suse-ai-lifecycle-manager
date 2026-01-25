<script lang="ts" setup>
import { defineProps, withDefaults, ref, computed, onMounted, getCurrentInstance, watch } from 'vue';
import yaml from 'js-yaml';
import { Banner } from '@components/Banner';
import Loading from '@shell/components/Loading';
import AsyncButton from '@shell/components/AsyncButton';
import BasicInfoStep from './wizard/BasicInfoStep.vue';
import TargetStep from './wizard/TargetStep.vue';
import ValuesStep from './wizard/ValuesStep.vue';
import ReviewStep from './wizard/ReviewStep.vue';
import InstallProgressModal, { type ClusterInstallProgress } from './wizard/InstallProgressModal.vue';
import {
  findChartInRepo,
  ensureNamespace,
  createOrUpgradeApp,
  listChartVersions,
  fetchChartDefaultValues,
  fetchChartYaml,
  ensureRegistrySecretSimple,
  ensureServiceAccountPullSecret,
  ensurePullSecretOnAllSAs,
  waitForAppInstall,
  getClusters,
  getInstalledHelmDetails,
  inferClusterRepoForChart,
  listClusterRepos
} from '../../services/rancher-apps';
import { getRepoAuthForClusterRepo } from '../../services/repo-auth';
import { persistLoad, persistSave, persistClear } from '../../services/ui-persist';
import { fetchSuseAiApps, getClusterRepoNameFromUrl } from '../../services/app-collection';

const REPO_CLUSTER = 'local' as const;

type WizardMode = 'install' | 'manage';

type WizardForm = {
  release: string;
  namespace: string;
  clusters: string[];
  chartRepo: string;
  chartName: string;
  chartVersion: string;
  values: Record<string, any>;
};

interface Props {
  slug: string;
  mode?: WizardMode;
}

const props = withDefaults(defineProps<Props>(), {
  mode: 'install'
});

const vm = getCurrentInstance()!.proxy as any;
const store = vm.$store;
const router = vm.$router;
const route = vm.$route;

const loading = ref(true);
const submitting = ref(false);
const error = ref<string | null>(null);
const versions = ref<string[]>([]);
const loadingVersions = ref(false);
const loadingValues = ref(false);
const versionInfo = ref<any | null>(null);
const questionsLoading = ref(false);
const versionInfoKey = ref('');
const defaultValuesSnapshot = ref<Record<string, any>>({});

// Multi-cluster install progress state
const showProgressModal = ref(false);
const installProgress = ref<ClusterInstallProgress[]>([]);

const PKEY = `${props.mode}.${props.slug}`;
const TTL = 1000 * 60 * 60;

const form = ref<WizardForm>({
  release: props.slug,
  namespace: `${props.slug}-system`,
  clusters: [],
  chartRepo: '',
  chartName: props.slug,
  chartVersion: '',
  values: {}
});

// Mode and version computed properties - must be declared before wizardSteps
const isInstallMode = computed(() => props.mode === 'install');
const isManageMode = computed(() => props.mode === 'manage');

const versionOptions = computed(() =>
  (versions.value || []).map(v => ({ label: v, value: v }))
);

// Wizard step configuration for Rancher Wizard component
const wizardSteps = computed(() => [
  {
    name: 'basic-info',
    label: 'Basic Information',
    ready: true,
    weight: 1
  },
  {
    name: 'target',
    label: 'Target Cluster',
    ready: !!form.value.chartRepo && !!form.value.chartVersion && !loadingVersions.value,
    weight: 2
  },
  {
    name: 'values',
    label: 'Configuration',
    ready: form.value.clusters.length > 0 && !loadingValues.value,
    weight: 3
  },
  {
    name: 'review',
    label: 'Review',
    ready: form.value.clusters.length > 0,
    weight: 4
  }
]);

const currentStep = ref(0);

// Restore persisted state
const saved = persistLoad<{ step?: number; form?: Partial<WizardForm> }>(PKEY, {}, TTL);
if (saved.form) Object.assign(form.value, saved.form);
if (typeof saved.step === 'number') currentStep.value = Math.min(saved.step, wizardSteps.value.length - 1);

const wizardTitle = computed(() => isInstallMode.value ? 'Install' : 'Manage');
const inStore = computed(() => store?.getters?.currentStore?.() || 'cluster');
const PSP_VARIABLE_MAP: Record<string, string> = {
  epinio:                     'global.rbac.pspEnabled',
  longhorn:                   'enablePSP',
  'rancher-alerting-drivers': 'global.cattle.psp.enabled',
  neuvector:                  'global.cattle.psp.enabled',
  'prometheus-federator':     'global.rbac.pspEnabled'
};
const ignoreVariables = computed(() => {
  const key = PSP_VARIABLE_MAP[form.value.chartName as keyof typeof PSP_VARIABLE_MAP];

  return key ? [key] : [];
});
const hasQuestions = computed(() => !!versionInfo.value?.questions);

watch(() => [form.value.chartRepo, form.value.chartName, form.value.chartVersion], () => {
  versionInfo.value = null;
  versionInfoKey.value = '';
  defaultValuesSnapshot.value = {};
});

// Basic info form computed
const basicInfoForm = computed({
  get: () => ({
    release: form.value.release,
    namespace: form.value.namespace,
    chartRepo: form.value.chartRepo,
    chartName: form.value.chartName,
    chartVersion: form.value.chartVersion
  }),
  set: async (value) => {
    const oldRepo = form.value.chartRepo;
    const oldVersion = form.value.chartVersion;

    form.value.release = value.release;
    form.value.namespace = value.namespace;
    form.value.chartRepo = value.chartRepo;
    form.value.chartName = value.chartName;
    form.value.chartVersion = value.chartVersion;

    // Refresh versions if repo changed
    if (oldRepo !== value.chartRepo) {
      await refreshVersions();
    }

    // Load default values if version changed in install mode
    if (oldVersion !== value.chartVersion) {
      await onVersionChange();
    }
  }
});

onMounted(async () => {
  try {
    await initializeWizard();
  } catch (e) {
    error.value = `Failed to initialize: ${e.message || 'Unknown error'}`;
  } finally {
    loading.value = false;
  }
});

async function initializeWizard() {
  // Populate from URL parameters first
  populateFromUrlParams();

  if (isInstallMode.value) {
    await initializeInstallMode();
  } else {
    await initializeManageMode();
  }

  await refreshVersions();
}

function populateFromUrlParams() {
  const query = route?.query || {};

  if (isInstallMode.value) {
    // For install mode, use simple pre-population
    form.value.release = props.slug;
    form.value.namespace = `${props.slug}-system`;
    form.value.chartRepo = query.repo as string || '';
    form.value.chartName = props.slug;
  } else {
    // For manage mode, get from URL path parameters
    form.value.release = query.instanceName as string || props.slug;
    form.value.namespace = query.instanceNamespace as string || `${props.slug}-system`;
    const instanceCluster = query.instanceCluster as string || '';
    if (instanceCluster) {
      form.value.clusters = [instanceCluster];
    }
  }
}

async function initializeInstallMode() {
  if (!store) return;

  // If repo is provided in query, use it
  if (form.value.chartRepo) {
    const guess = await findChartInRepo(store, REPO_CLUSTER, form.value.chartRepo, props.slug);
    if (guess) {
      form.value.chartName = guess.chartName;
      form.value.chartVersion = guess.version;
      return;
    }
  }

  // Try to find the app in SUSE AI apps collection
  const repo = await findRepoForApp(props.slug);
  if (repo) {
    form.value.chartRepo = repo;
    const guess = await findChartInRepo(store, REPO_CLUSTER, repo, props.slug);
    if (guess) {
      form.value.chartName = guess.chartName;
      form.value.chartVersion = guess.version;
    }
  }
}

async function findRepoForApp(slug: string): Promise<string | null> {
  if (!store) return null;

  try {
    // First try SUSE AI apps
    const suseAiApps = await fetchSuseAiApps(store);
    const staticApp = suseAiApps.find(app => app.slug_name === slug);

    if (staticApp?.repository_url) {
      const clusterRepoName = await getClusterRepoNameFromUrl(store, staticApp.repository_url);
      if (clusterRepoName) return clusterRepoName;
    }

    // Fallback to inference
    return await inferClusterRepoForChart(store, slug);
  } catch (e) {
    console.warn('Failed to find repo for app:', e);
    return null;
  }
}

async function initializeManageMode() {
  if (!store) throw new Error('Store not available');

  const targetCluster = form.value.clusters[0];

  if (!targetCluster) {
    throw new Error('No cluster specified for manage mode');
  }

  // Load app details from the target cluster
  // If app doesn't exist, loadInstalledAppDetails will throw an error
  await loadInstalledAppDetails(targetCluster);
}

async function loadInstalledAppDetails(clusterId: string) {
  let foundValues = false;
  if (!store) return;

  try {
    const helmDetails = await getInstalledHelmDetails(store, clusterId, form.value.namespace, form.value.release);

    // Verify we got valid data - if app doesn't exist, chartName would be missing
    if (!helmDetails.chartName) {
      throw new Error(`App ${form.value.release} not found in cluster ${clusterId}`);
    }

    if (helmDetails.chartName) form.value.chartName = helmDetails.chartName;
    if (helmDetails.chartVersion) {
      form.value.chartVersion = helmDetails.chartVersion;
    }

    if (helmDetails.values && Object.keys(helmDetails.values).length > 0) {
      form.value.values = helmDetails.values;
      foundValues = true;
    }
  } catch (helmError) {
    console.warn('Failed to load app details from Helm:', helmError);
    throw helmError; // Re-throw to fail fast if app doesn't exist
  }

  // Infer repo if still unknown
  if (!form.value.chartRepo && form.value.chartName) {
    try {
      const repo = await inferClusterRepoForChart(store, form.value.chartName);
      if (repo) form.value.chartRepo = repo;
    } catch (e) {
      console.warn('Failed to infer repository:', e);
    }
  }

  // Log final results for debugging
  if (!foundValues) {
    console.warn('No values found for installed app:', {
      cluster: clusterId,
      namespace: form.value.namespace,
      release: form.value.release
    });
  }
}

async function refreshVersions() {
  if (!store || !form.value.chartRepo || !form.value.chartName) return;

  loadingVersions.value = true;
  try {
    const vs = await listChartVersions(store, REPO_CLUSTER, form.value.chartRepo, form.value.chartName);
    versions.value = Array.from(new Set(vs));

    // Set initial version if not already set
    if (!form.value.chartVersion && vs.length > 0) {
      form.value.chartVersion = vs[0];
    }

    if (form.value.chartVersion) {
      await ensureVersionInfoLoaded();

      // Load default values for install mode
      if (isInstallMode.value) {
        await loadDefaultValues({ skipVersionInfoFetch: true });
      }
    }
  } finally {
    loadingVersions.value = false;
  }
}

async function loadDefaultValues(options: { skipVersionInfoFetch?: boolean } = {}) {
  if (!store || !form.value.chartRepo || !form.value.chartName || !form.value.chartVersion) {
    return;
  }

  loadingValues.value = true;
  error.value = null;

  try {
    if (!options.skipVersionInfoFetch) {
      await ensureVersionInfoLoaded();
    }

    let baseValues: Record<string, any> | null = defaultValuesSnapshot.value && Object.keys(defaultValuesSnapshot.value).length
      ? defaultValuesSnapshot.value
      : null;

    if (!baseValues && versionInfo.value?.values) {
      baseValues = JSON.parse(JSON.stringify(versionInfo.value.values || {}));
      defaultValuesSnapshot.value = baseValues;
    }

    if (baseValues) {
      form.value.values = JSON.parse(JSON.stringify(baseValues));
    } else {
      const valuesText = await fetchChartDefaultValues(
        store,
        REPO_CLUSTER,
        form.value.chartRepo,
        form.value.chartName,
        form.value.chartVersion
      );

      if (valuesText?.trim()) {
        const parsed = (yaml.load(valuesText) as any) || {};
        defaultValuesSnapshot.value = JSON.parse(JSON.stringify(parsed));
        form.value.values = parsed;
      } else {
        error.value = 'No default values found for the selected version.';
      }
    }
  } catch (e: any) {
    error.value = e?.message || 'Failed to fetch default values.';
  } finally {
    loadingValues.value = false;
  }
}

async function ensureVersionInfoLoaded() {
  if (!store) {
    versionInfo.value = null;
    versionInfoKey.value = '';
    return null;
  }

  const repo = form.value.chartRepo;
  const chart = form.value.chartName;
  const version = form.value.chartVersion;

  if (!repo || !chart || !version) {
    versionInfo.value = null;
    versionInfoKey.value = '';
    return null;
  }

  const key = `${repo}:::${chart}:::${version}`;

  if (versionInfo.value && versionInfoKey.value === key) {
    return versionInfo.value;
  }

  questionsLoading.value = true;
  try {
    await store.dispatch('catalog/load');
    const info = await store.dispatch('catalog/getVersionInfo', {
      repoType:    'cluster',
      repoName:    repo,
      chartName:   chart,
      versionName: version
    });

    versionInfo.value = info;
    versionInfoKey.value = key;
    defaultValuesSnapshot.value = JSON.parse(JSON.stringify(info?.values || {}));

    return info;
  } catch (e) {
    console.warn('[SUSE-AI] Failed to load chart version info', e);
    versionInfo.value = null;
    versionInfoKey.value = '';
    defaultValuesSnapshot.value = {};

    return null;
  } finally {
    questionsLoading.value = false;
  }
}

// Handle version changes for install mode
async function onVersionChange() {
  if (!form.value.chartVersion) {
    versionInfo.value = null;
    versionInfoKey.value = '';
    return;
  }

  await ensureVersionInfoLoaded();

  if (isInstallMode.value) {
    await loadDefaultValues({ skipVersionInfoFetch: true });
  }

  // Persist form state
  persistSave(PKEY, { step: currentStep.value, form: form.value });
}

// Simplified event handlers
function onValuesEdited() {
  // Persist form state when values are edited
  persistSave(PKEY, { step: currentStep.value, form: form.value });
}

// Wizard event handlers
function onWizardNext({ step }) {
  currentStep.value = step;
  persistSave(PKEY, { step: currentStep.value, form: form.value });
}

async function onWizardFinish() {
  await submit();
}

function onWizardCancel() {
  persistClear(PKEY);
  // Redirect to AppInstances page instead of Apps page
  router?.push({
    name: `c-cluster-suseai-app-instances`,
    params: {
      cluster: route?.params?.cluster,
      slug: props.slug
    },
    query: {
      repo: route.query.repo
    }
  });
}

async function submit() {
  try {
    submitting.value = true;
    error.value = null;

    if (!form.value.chartRepo || !form.value.chartName || !form.value.chartVersion) {
      error.value = 'Please set repository, chart and version.'; return;
    }

    if (form.value.clusters.length === 0) {
      error.value = 'Please select at least one cluster.'; return;
    }

    if (!store) { error.value = 'Store not available'; return; }

    const actionLabel = isInstallMode.value ? 'INSTALL' : 'UPGRADE';
    const targetClusters = form.value.clusters;

    console.log(`[SUSE-AI] ${actionLabel} start `, {
      clusters: targetClusters,
      ns: form.value.namespace,
      release: form.value.release
    });

    if (isInstallMode.value) {
      await performMultiClusterInstall();
    } else {
      await performUpgrade();
      // For upgrade, redirect immediately
      navigateToAppInstances();
    }
  } catch (e: any) {
    error.value = e?.message || 'Operation failed';
    submitting.value = false;
  }
}

function navigateToAppInstances() {
  persistClear(PKEY);
  router?.push({
    name: `c-cluster-suseai-app-instances`,
    params: {
      cluster: route?.params?.cluster,
      slug: props.slug
    },
    query: {
      repo: route.query.repo
    }
  });
}

// Get cluster name for display (used in progress modal)
async function getClusterDisplayName(clusterId: string): Promise<string> {
  try {
    const clusters = await getClusters(store);
    const cluster = clusters.find((c: any) => c.id === clusterId);
    return cluster?.name || clusterId;
  } catch {
    return clusterId;
  }
}

// Concurrency limit for parallel installations
const INSTALL_CONCURRENCY = 3;

// Multi-cluster install orchestration with parallel execution
async function performMultiClusterInstall() {
  const targetClusters = form.value.clusters;

  // Initialize progress for all clusters
  installProgress.value = await Promise.all(
    targetClusters.map(async (clusterId) => ({
      clusterId,
      clusterName: await getClusterDisplayName(clusterId),
      status: 'pending' as const,
      progress: 0,
      message: 'Waiting to start...'
    }))
  );

  showProgressModal.value = true;

  // Install to clusters in parallel with concurrency limit
  await installWithConcurrencyLimit(targetClusters, INSTALL_CONCURRENCY);

  // Check final status
  const allSucceeded = installProgress.value.every(p => p.status === 'success');
  if (allSucceeded) {
    console.log('[SUSE-AI] Multi-cluster install completed successfully');
  } else {
    const failed = installProgress.value.filter(p => p.status === 'failed');
    console.warn(`[SUSE-AI] Multi-cluster install completed with ${failed.length} failure(s)`);
  }

  submitting.value = false;
}

// Install to multiple clusters with concurrency limit
async function installWithConcurrencyLimit(clusterIds: string[], concurrency: number) {
  const queue = [...clusterIds];
  const executing: Promise<void>[] = [];

  while (queue.length > 0 || executing.length > 0) {
    // Start new installations up to concurrency limit
    while (queue.length > 0 && executing.length < concurrency) {
      const clusterId = queue.shift()!;
      const promise = installSingleCluster(clusterId).then(() => {
        // Remove from executing when done
        const index = executing.indexOf(promise);
        if (index > -1) executing.splice(index, 1);
      });
      executing.push(promise);
    }

    // Wait for at least one to complete before continuing
    if (executing.length > 0) {
      await Promise.race(executing);
    }
  }
}

// Install to a single cluster and update progress
async function installSingleCluster(clusterId: string): Promise<void> {
  updateClusterProgress(clusterId, {
    status: 'installing',
    progress: 10,
    message: 'Starting installation...'
  });

  try {
    await installToCluster(clusterId, (progress, message) => {
      updateClusterProgress(clusterId, { progress, message });
    });

    updateClusterProgress(clusterId, {
      status: 'success',
      progress: 100,
      message: 'Installation completed successfully'
    });
  } catch (e: any) {
    updateClusterProgress(clusterId, {
      status: 'failed',
      progress: 0,
      message: 'Installation failed',
      error: e?.message || 'Unknown error'
    });
  }
}

// Update progress for a specific cluster
function updateClusterProgress(clusterId: string, updates: Partial<ClusterInstallProgress>) {
  const index = installProgress.value.findIndex(p => p.clusterId === clusterId);
  if (index !== -1) {
    installProgress.value[index] = { ...installProgress.value[index], ...updates };
  }
}

// Progress modal event handlers
function onProgressModalDone() {
  showProgressModal.value = false;
  navigateToAppInstances();
}

function onProgressModalCancel() {
  showProgressModal.value = false;
  submitting.value = false;
}

async function onProgressModalRetryAll() {
  // Reset all to pending and retry
  installProgress.value = installProgress.value.map(p => ({
    ...p,
    status: 'pending' as const,
    progress: 0,
    message: 'Waiting to retry...',
    error: undefined
  }));

  submitting.value = true;

  // Re-run installation with parallelization
  const clusterIds = installProgress.value.map(p => p.clusterId);
  await installWithConcurrencyLimit(clusterIds, INSTALL_CONCURRENCY);

  submitting.value = false;
}

async function onProgressModalRetryFailed() {
  const failedClusters = installProgress.value.filter(p => p.status === 'failed');

  // Reset failed clusters to pending
  for (const item of failedClusters) {
    updateClusterProgress(item.clusterId, {
      status: 'pending',
      progress: 0,
      message: 'Waiting to retry...',
      error: undefined
    });
  }

  submitting.value = true;

  // Retry failed clusters with parallelization
  const failedIds = failedClusters.map(p => p.clusterId);
  await installWithConcurrencyLimit(failedIds, INSTALL_CONCURRENCY);

  submitting.value = false;
}

function onProgressModalContinueAnyway() {
  showProgressModal.value = false;
  navigateToAppInstances();
}

// Install to a single cluster with progress callback
async function installToCluster(
  clusterId: string,
  onProgress: (progress: number, message: string) => void
) {
  const allPullSecrets = new Set<string>();

  onProgress(15, 'Preparing namespace...');

  // Resolve creds from SELECTED ClusterRepo
  const repoCtx = await getRepoAuthForClusterRepo(store, form.value.chartRepo);
  const desiredSecretBase = repoCtx.secretName || `repo-${form.value.chartRepo}`;
  const hasRepoCredentials = !!repoCtx.auth?.username && !!repoCtx.auth?.password;

  await ensureNamespace(store, clusterId, form.value.namespace);

  onProgress(25, 'Setting up registry credentials...');

  if (hasRepoCredentials) {
    try {
      const finalSecretName = await ensureRegistrySecretSimple(
        store, clusterId, form.value.namespace,
        repoCtx.registryHost, desiredSecretBase,
        repoCtx.auth!.username, repoCtx.auth!.password
      );
      if (finalSecretName) {
        allPullSecrets.add(finalSecretName);
      }
    } catch (e: any) {
      console.error('[SUSE-AI] pull-secret creation skipped:', e?.message || e);
    }
  }

  onProgress(35, 'Processing chart dependencies...');

  // Handle subchart dependencies
  const chartYaml = await fetchChartYaml(store, form.value.chartRepo, form.value.chartName, form.value.chartVersion);
  if (chartYaml?.dependencies) {
    // Fetch cluster repos 
    let clusterRepos: any[] = [];
    try {
      clusterRepos = await listClusterRepos(store);
    } catch (e: any) {
      console.warn('[SUSE-AI] Failed to list cluster repos for dependency secrets (continuing without):', e?.message || e);
    }

    // Include spec.url, spec.gitRepo, and spec.ociRepo for matching
    // Git-backed and OCI repos use different spec fields
    const repos = clusterRepos
      .filter((r: any) => r?.metadata?.name && (r?.spec?.url || r?.spec?.gitRepo || r?.spec?.ociRepo))
      .map((r: any) => ({
        name: r.metadata.name,
        url: r.spec.url || r.spec.gitRepo || r.spec.ociRepo
      }));
    const processedRepos = new Set<string>();

    for (const dep of chartYaml.dependencies) {
      if (!dep.repository) continue;

      const normalizeUrl = (url: string) => url?.replace(/\/+$/, '').toLowerCase();
      const repo = repos.find((r: any) => normalizeUrl(r.url) === normalizeUrl(dep.repository));

      if (!repo) {
        console.warn(`[SUSE-AI] Subchart ${dep.name} repository ${dep.repository} not found in configured ClusterRepos`);
        continue;
      }

      if (processedRepos.has(repo.name)) continue;
      processedRepos.add(repo.name);

      const subRepoCtx = await getRepoAuthForClusterRepo(store, repo.name);
      if (!subRepoCtx.auth?.username || !subRepoCtx.auth?.password) continue;

      try {
        const subDesiredSecretBase = subRepoCtx.secretName || `repo-${repo.name}`;
        const finalSecretName = await ensureRegistrySecretSimple(
          store, clusterId, form.value.namespace,
          subRepoCtx.registryHost, subDesiredSecretBase,
          subRepoCtx.auth.username, subRepoCtx.auth.password
        );
        if (finalSecretName) {
          allPullSecrets.add(finalSecretName);
        }
      } catch (e: any) {
        console.warn(`[SUSE-AI] Pull secret creation skipped for subchart ${dep.name}:`, e?.message || e);
      }
    }
  }

  onProgress(45, 'Configuring image pull secrets...');

  const v = JSON.parse(JSON.stringify(form.value.values || {}));
  const pullSecrets = Array.from(allPullSecrets);

  // Only add imagePullSecrets if we have a secret name (i.e., repo has authentication)
  if (pullSecrets.length > 0) {
    const addSecrets = (arr: any): any[] => {
      const list = Array.isArray(arr) ? arr.slice() : [];
      for (const secretName of pullSecrets) {
        const hasStr = list.some((e: any) => e === secretName);
        const hasObj = list.some((e: any) => e && typeof e === 'object' && e.name === secretName);
        if (!hasStr && !hasObj) {
          list.push({ name: secretName });
        }
      }
      return list;
    };
    v.global = v.global || {};
    v.global.imagePullSecrets = addSecrets(v.global.imagePullSecrets);
    v.imagePullSecrets = addSecrets(v.imagePullSecrets);

    const saCandidates = new Set<string>(['default']);
    const vs = (v as any).serviceAccount || {};
    if (typeof vs?.name === 'string' && vs.name.trim()) saCandidates.add(vs.name.trim());
    else if (vs.create === undefined || !!vs.create) saCandidates.add(form.value.release);
    for (const sa of saCandidates) {
      for (const secretName of pullSecrets) {
        try { await ensureServiceAccountPullSecret(store, clusterId, form.value.namespace, sa, secretName); }
        catch (e) { console.warn('[SUSE-AI] SA pull-secret attach (pre) failed', { sa, ns: form.value.namespace, e }); }
      }
    }
  }

  onProgress(55, 'Installing Helm chart...');

  console.log('[SUSE-AI] calling install ', {
    cluster: clusterId,
    repo: form.value.chartRepo,
    chart: form.value.chartName,
    version: form.value.chartVersion,
    ns: form.value.namespace,
    release: form.value.release,
    values: v
  });

  await createOrUpgradeApp(
    store, clusterId, form.value.namespace, form.value.release,
    { repoName: form.value.chartRepo, chartName: form.value.chartName, version: form.value.chartVersion },
    v,
    'install'
  );

  onProgress(75, 'Waiting for app deployment...');

  try {
    await waitForAppInstall(store, clusterId, form.value.namespace, form.value.release, 180_000);
  } catch (e: any) {
    console.error('[SUSE-AI] post-install app status (peek): ', { error: e?.message || e });
    throw new Error(`App resource did not appear in namespace ${form.value.namespace}. Check Rancher logs and ClusterRepo permissions.`);
  }

  onProgress(90, 'Finalizing service accounts...');

  if (pullSecrets.length > 0) {
    for (let attempt = 1; attempt <= 5; attempt++) {
      try {
        for (const secretName of pullSecrets) {
          await ensurePullSecretOnAllSAs(store, clusterId, form.value.namespace, secretName);
        }
        break;
      } catch (e) {
        if (attempt === 5) break;
        await new Promise(r => setTimeout(r, 2000));
      }
    }
  }

  onProgress(100, 'Installation complete');
}

async function performUpgrade() {
  for (const cid of form.value.clusters) {
    await ensureNamespace(store, cid, form.value.namespace);
    await createOrUpgradeApp(
      store, cid, form.value.namespace, form.value.release,
      { repoName: form.value.chartRepo, chartName: form.value.chartName, version: form.value.chartVersion },
      form.value.values,
      'upgrade'
    );
  }
}

// Custom wizard navigation methods
function goToStep(stepIndex: number) {
  // Only allow navigation if step is ready or going backwards
  if (stepIndex <= currentStep.value || wizardSteps.value[stepIndex].ready) {
    currentStep.value = stepIndex;
  }
}

function nextStep() {
  if (currentStep.value < wizardSteps.value.length - 1 && wizardSteps.value[currentStep.value + 1].ready) {
    currentStep.value++;
  }
}

function previousStep() {
  if (currentStep.value > 0) {
    currentStep.value--;
  }
}
</script>

<template>
  <div class="install-steps pt-20 outlet">
    <Loading v-if="loading" />
    
    <div v-else class="custom-wizard">
      <!-- Fixed Header -->
      <div class="wizard-header">
        <h1>{{ (route.query.n as string) || props.slug }}</h1>
        <p class="text-muted">{{ wizardTitle }}</p>
      </div>

      <!-- Fixed Step Navigation -->
      <div class="wizard-nav">
        <div class="steps-container">
          <div 
            v-for="(step, index) in wizardSteps" 
            :key="step.name"
            class="step-item"
            :class="{
              'active': index === currentStep,
              'completed': index < currentStep,
              'disabled': !step.ready && index > currentStep
            }"
            @click="goToStep(index)"
          >
            <div class="step-number">
              <i v-if="index < currentStep" class="icon icon-checkmark" />
              <span v-else>{{ index + 1 }}</span>
            </div>
            <div class="step-label">{{ step.label }}</div>
          </div>
        </div>
      </div>

      <!-- Scrollable Content Area -->
      <div class="wizard-content-wrapper">
        <!-- Error Banner -->
        <Banner v-if="error" color="error" class="mb-20">
          {{ error }}
        </Banner>

        <!-- Step Content -->
        <div class="wizard-content">
          <!-- Step: Basic Information -->
          <BasicInfoStep
            v-if="currentStep === 0"
            v-model:form="basicInfoForm"
            :version-options="versionOptions"
            :loading-versions="loadingVersions"
          />

          <!-- Step: Target Cluster -->
          <TargetStep
            v-else-if="currentStep === 1"
            :mode="props.mode"
            v-model:clusters="form.clusters"
            :app-slug="props.slug"
            :app-name="(route.query.n as string) || props.slug"
          />

          <!-- Step: Configuration -->
          <ValuesStep
            v-else-if="currentStep === 2"
            v-model:values="form.values"
            :chart-repo="form.chartRepo"
            :chart-name="form.chartName"
            :chart-version="form.chartVersion"
            :loading-values="loadingValues"
            :version-dirty="false"
            :has-questions="hasQuestions"
            :questions-source="versionInfo"
            :questions-loading="questionsLoading"
            :ignore-variables="ignoreVariables"
            :target-namespace="form.namespace"
            :mode="props.mode"
            :in-store="inStore"
            @load-defaults="loadDefaultValues"
            @values-edited="onValuesEdited"
          />

          <!-- Step: Review -->
          <ReviewStep
            v-else-if="currentStep === 3"
            :mode="props.mode"
            :release="form.release"
            :namespace="form.namespace"
            :chart-repo="form.chartRepo"
            :chart-name="form.chartName"
            :chart-version="form.chartVersion"
            :clusters="form.clusters"
            v-model:values="form.values"
            @values-edited="onValuesEdited"
          />
        </div>
      </div>

      <!-- Fixed Bottom Navigation Buttons -->
      <div class="wizard-buttons-fixed">
        <button
          v-if="currentStep > 0"
          class="btn role-secondary"
          @click="previousStep"
        >
          Previous
        </button>

        <div class="flex-spacer" />

        <button
          class="btn role-secondary mr-10"
          @click="onWizardCancel"
        >
          Cancel
        </button>

        <button
          v-if="currentStep === 0 || currentStep === 1 || currentStep === 2"
          class="btn role-primary"
          :disabled="!wizardSteps[currentStep].ready"
          @click="nextStep"
        >
          Next
        </button>

        <button
          v-else-if="currentStep === 3"
          class="btn role-primary"
          :disabled="!wizardSteps[currentStep].ready || submitting"
          @click="onWizardFinish"
        >
          <i v-if="submitting" class="icon icon-spinner icon-spin mr-5" />
          <span v-if="submitting">
            {{ props.mode === 'install' ? 'Installing...' : 'Saving...' }}
          </span>
          <span v-else>
            {{ props.mode === 'install' ? 'Install' : 'Save' }}
          </span>
        </button>
      </div>
    </div>

    <!-- Multi-cluster Install Progress Modal -->
    <InstallProgressModal
      :show="showProgressModal"
      :progress="installProgress"
      :title="`Installing ${(route.query.n as string) || props.slug}`"
      @done="onProgressModalDone"
      @cancel="onProgressModalCancel"
      @retry-all="onProgressModalRetryAll"
      @retry-failed="onProgressModalRetryFailed"
      @continue-anyway="onProgressModalContinueAnyway"
    />
  </div>
</template>


<style scoped>
/* Ensure Loading overlay inherits Rancher's standard background */
.install-steps {
  --overlay-bg: var(--body-bg, #ffffff);
}

/* Button utilities */
.mr-5 {
  margin-right: 5px;
}

.icon-spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* Main wizard container - matches Rancher's main content areas */
.custom-wizard {
  background: var(--body-bg, #ffffff);
  max-width: 100%;
  width: 100%;
  height: calc(100vh - 140px);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* Header styling to match Rancher's page headers */
.wizard-header {
  flex-shrink: 0;
  padding: 20px 24px 16px 24px;
  background: var(--body-bg, #ffffff);
}

.wizard-header h1 {
  margin: 0 0 4px 0;
  font-size: 20px;
  font-weight: 600;
  color: var(--body-text, #111827);
  line-height: 1.2;
}

.wizard-header p {
  margin: 0;
  font-size: 14px;
  color: var(--muted, #6b7280);
  font-weight: 400;
}

/* Step navigation - matches Rancher's wizard pattern */
.wizard-nav {
  flex-shrink: 0;
  width: 100%;
  padding: 20px 24px;
  background: var(--body-bg, #ffffff);
}

.steps-container {
  display: flex;
  justify-content: space-between;
  position: relative;
  max-width: 100%;
  align-items: center;
}

.steps-container::before {
  content: '';
  position: absolute;
  top: 20px;
  left: 50px;
  right: 50px;
  height: 1px;
  background: var(--border, #f3f4f6);
  z-index: 0;
}

.step-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  cursor: pointer;
  flex: 1;
  max-width: 200px;
  position: relative;
  z-index: 1;
  transition: all 0.2s ease;
}

.step-item.disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.step-item:hover:not(.disabled) .step-number {
  transform: scale(1.05);
}

.step-number {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: var(--body-bg, #ffffff);
  border: 1px solid var(--border, #f3f4f6);
  color: var(--muted, #9ca3af);
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 500;
  font-size: 14px;
  margin-bottom: 8px;
  transition: all 0.2s ease;
}

.step-item.active .step-number {
  background: var(--primary, #2563eb);
  border-color: var(--primary, #2563eb);
  color: white;
}

.step-item.completed .step-number {
  background: var(--success, #16a34a);
  border-color: var(--success, #16a34a);
  color: white;
}

.step-label {
  font-size: 13px;
  text-align: center;
  color: var(--muted, #6b7280);
  font-weight: 400;
  line-height: 1.3;
}

.step-item.active .step-label {
  color: var(--primary, #2563eb);
  font-weight: 500;
}

.step-item.completed .step-label {
  color: var(--body-text, #111827);
}

/* Content area - matches Rancher's form containers */
.wizard-content-wrapper {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  background: var(--body-bg, #ffffff);
}

.wizard-content {
  padding: 24px;
  background: var(--body-bg, #ffffff);
  margin: 0;
  min-height: 100%;
}

/* Bottom button bar - matches Rancher's action bars */
.wizard-buttons-fixed {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  width: 100%;
  padding: 16px 24px;
  background: var(--body-bg, #ffffff);
}

.flex-spacer {
  flex: 1;
}

/* Button overrides to match Rancher's button styling */
.wizard-buttons-fixed .btn {
  height: 36px;
  padding: 0 16px;
  font-size: 14px;
  font-weight: 500;
  border-radius: 4px;
  transition: all 0.2s ease;
}

.wizard-buttons-fixed .btn.role-secondary,
.wizard-buttons-fixed .btn.role-secondary:focus,
.wizard-buttons-fixed .btn.role-secondary:active {
  background: var(--body-bg, #ffffff);
  border: 1px solid var(--border, #d1d5db);
  color: var(--body-text, #111827) !important;
  box-shadow: none;
}

.wizard-buttons-fixed .btn.role-secondary:hover {
  background: var(--body-bg, #ffffff);
  border-color: var(--border-hover, #9ca3af);
  color: var(--body-text, #111827) !important;
}

.wizard-buttons-fixed .btn.role-secondary:focus-visible {
  outline: 2px solid var(--primary, #2563eb);
  outline-offset: 2px;
}

.wizard-buttons-fixed .btn.role-primary {
  background: var(--primary, #2563eb);
  border: 1px solid var(--primary, #2563eb);
  color: white;
}

.wizard-buttons-fixed .btn.role-primary:hover:not(:disabled) {
  background: var(--primary-hover, #1d4ed8);
  border-color: var(--primary-hover, #1d4ed8);
}

.wizard-buttons-fixed .btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

/* Utility classes */
.mb-20 {
  margin-bottom: 20px;
}
</style>
