# Runbook — evaluate downstream pull-secret delivery

End-to-end setup for testing the `inject-nvidia-auth` branch from a fresh
download of the Rancher kubeconfig. Designed to be copy-pasteable into a
**fish** shell, top to bottom. Assumes:

- Rancher management cluster reachable via downloaded kubeconfig.
- At least one downstream cluster registered with Rancher, **with a
  default StorageClass** (see "Downstream cluster prerequisites" at the
  end of this section).
- `docker`, `helm`, `kubectl`, `jq`, `go` (1.25+), `git` installed locally.
- You are authenticated to `ghcr.io` as `thbertoldi`
  (`docker login ghcr.io -u thbertoldi`).
- The repo is checked out at
  `/home/thbertoldi/suse/suse-ai-lifecycle-manager` on the
  `inject-nvidia-auth` branch.

---

## 0. Set the per-session variables

Edit these to match today's download + the downstream cluster ID, then
paste the rest verbatim.

```fish
# Today's kubeconfig (the number after "local" changes each download)
set KCFG '/home/thbertoldi/Downloads/local (XX).yaml'

# Downstream cluster ID to target — list them with the command on the
# next line; pick the one that's NOT "local" and is in your test env.
set DOWNSTREAM c-msmks

# Quick sanity check + list downstream clusters
kubectl --kubeconfig "$KCFG" get clusters.management.cattle.io \
  -o custom-columns=ID:.metadata.name,NAME:.spec.displayName,STATE:.status.conditions[?\(@.type==\"Ready\"\)].status
```

### Downstream cluster prerequisites

Before any chart with persistent storage can deploy, the downstream
cluster needs **a default StorageClass**. Most stock RKE2/k3s clusters
ship `local-path` (provisioner `rancher.io/local-path`) but do not mark
it default. Without a default, every PVC the chart creates stays
unbound, every dependent pod gets the scheduler error
`0/N nodes are available: pod has unbound immediate PersistentVolumeClaims. not found`,
and the install never makes it past Pending.

Check:
```fish
kubectl --kubeconfig "$KCFG" get --raw "/k8s/clusters/$DOWNSTREAM/apis/storage.k8s.io/v1/storageclasses" \
  | jq -r '.items[] | "\(.metadata.name) default=\(.metadata.annotations."storageclass.kubernetes.io/is-default-class" // "no")"'
```

If nothing is marked default, patch via the Rancher proxy (the in-cluster
kubectl can't `patch` through proxy URLs; curl with the kubeconfig's
bearer token does work):

```fish
set TOKEN (kubectl --kubeconfig "$KCFG" config view --raw --minify -o json | jq -r '.users[0].user.token')
set SERVER (kubectl --kubeconfig "$KCFG" config view --raw --minify -o json | jq -r '.clusters[0].cluster.server' | sed 's|/k8s/clusters/.*|/k8s/clusters/'$DOWNSTREAM'|')

curl -sS -k -X PATCH \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/merge-patch+json" \
  -d '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}' \
  "$SERVER/apis/storage.k8s.io/v1/storageclasses/local-path" \
  | jq '{name: .metadata.name, default: .metadata.annotations."storageclass.kubernetes.io/is-default-class"}'
```

Caveat: `local-path` pins PVs to the node where the pod first runs — fine
for testing, not for production. For multi-node persistence you'd want
Longhorn or another CSI driver.

---

## 1. Make sure your operator image is built and pushed

If you already have `ghcr.io/thbertoldi/aif-operator:dev-<timestamp>` from
a prior session, you can skip this step. Otherwise:

```fish
cd /home/thbertoldi/suse/suse-ai-lifecycle-manager/aif-operator
set TAG "dev-"(date +%s)
docker build -t "ghcr.io/thbertoldi/aif-operator:$TAG" \
             -t "ghcr.io/thbertoldi/aif-operator:dev" .
docker push "ghcr.io/thbertoldi/aif-operator:$TAG"
docker push "ghcr.io/thbertoldi/aif-operator:dev"
cd /home/thbertoldi/suse/suse-ai-lifecycle-manager

# Confirm GHCR package is public — otherwise the cluster needs an
# imagePullSecret to pull. Open in browser:
#   https://github.com/users/thbertoldi/packages/container/aif-operator/settings
# and set visibility to Public.
```

If `docker push` hangs with a `gpg: decryption failed: Timeout` (the
docker credential helper `pass` occasionally wedges):

```fish
# One-shot workaround: bypass the credstore by inlining the GHCR auth
# into ~/.docker/config.json temporarily.
set PASS (timeout 5 pass show docker-credential-helpers/Z2hjci5pbw==/thbertoldi 2>/dev/null | head -1)
set AUTH (echo -n "thbertoldi:$PASS" | base64 -w0)
cp ~/.docker/config.json ~/.docker/config.json.bak
jq --arg auth "$AUTH" '.auths["ghcr.io"].auth = $auth | del(.credsStore)' \
   ~/.docker/config.json.bak > ~/.docker/config.json
# … do your pushes …
mv ~/.docker/config.json.bak ~/.docker/config.json   # restore
```

---

## 2. Install the operator from the LOCAL branch chart (not the OCI one)

The published `oci://ghcr.io/suse/chart/aif-operator` ships an older CRD
that strips `status.pullSecretNames`/`source.app.vendor` and an older
ClusterRole that lacks `bundles` CRUD. Install from the branch chart
instead.

The `aiExtension.source.helm.values` block (added on this branch) pipes
through to the auto-installed aif-ui chart so you get the per-cluster CR
naming, vendor field, and `takeOwnership` behavior. Without the override
you'd get the upstream `ghcr.io/suse/aif-ui:0.1.0-dev.2` image, which
predates several wizard fixes and will create AIWorkloads named the same
on every cluster (immediate name collision on a second install).

```fish
helm install aif-operator charts/aif-operator/ \
  --kubeconfig "$KCFG" \
  --namespace aif-operator --create-namespace \
  --set manager.image.registry=ghcr.io \
  --set manager.image.repository=thbertoldi/aif-operator \
  --set manager.image.tag=dev \
  --set manager.image.pullPolicy=Always \
  --set aiExtension.source.helm.values.image.registry=ghcr.io \
  --set aiExtension.source.helm.values.image.repository=thbertoldi/aif-ui \
  --set aiExtension.source.helm.values.image.tag=0.1.0-dev.2 \
  --set aiExtension.source.helm.values.image.pullPolicy=Always

# Wait for rollout
kubectl --kubeconfig "$KCFG" -n aif-operator rollout status \
  deploy/aif-operator --timeout=120s

# The aif-ui Deployment is created asynchronously by the
# InstallAIExtension controller (~30s after operator pod is Ready).
# Confirm it picked up your image, not the upstream default:
kubectl --kubeconfig "$KCFG" -n cattle-ui-plugin-system get deploy aif-ui-server \
  -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
# Expected: ghcr.io/thbertoldi/aif-ui:0.1.0-dev.2
```

If you ever need to upgrade after editing the chart, remember Helm does
NOT update CRDs on upgrade — re-apply them by hand:

```fish
kubectl --kubeconfig "$KCFG" apply -f charts/aif-operator/crds/
helm upgrade aif-operator charts/aif-operator/ \
  --kubeconfig "$KCFG" --namespace aif-operator --reuse-values
```

---

## 3. Create the four Rancher ClusterRepos and their auth secrets

These power the SUSE + NVIDIA app catalogs in the UI. Without them, the
"Apps" page is empty even when the operator is healthy.

```fish
# 3a. Auth secrets in cattle-system (basic-auth format, NOT dockerconfigjson)

# NVIDIA NGC (both nvidia + nvidia-blueprint repos)
kubectl --kubeconfig "$KCFG" -n cattle-system create secret generic ngc-helm-auth \
  --type=kubernetes.io/basic-auth \
  --from-literal=username='$oauthtoken' \
  --from-literal=password='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'

# SUSE App Collection — note the password is the literal value below
# (it LOOKS base64-wrapped but isn't double-encoded; do NOT decode it)
kubectl --kubeconfig "$KCFG" -n cattle-system create secret generic app-collection-auth \
  --type=kubernetes.io/basic-auth \
  --from-literal=username='tbertoldi@suse.com' \
  --from-literal=password='Ynl1eXRya2VzdWRsaWJsZ2t6cm5sdGVybmNtcGt6Z3BpZmJieGlhdm9rY2t3dmlzcXB3Zm1vb2x4c2pkd2luZw=='

# SUSE Registry
kubectl --kubeconfig "$KCFG" -n cattle-system create secret generic suse-registry-auth \
  --type=kubernetes.io/basic-auth \
  --from-literal=username='regcode' \
  --from-literal=password='INTERNAL-USE-ONLY-c8d3-5629'

# 3b. ClusterRepo CRs (cluster-scoped)
echo '---
apiVersion: catalog.cattle.io/v1
kind: ClusterRepo
metadata: {name: application-collection}
spec:
  url: oci://dp.apps.rancher.io/charts
  clientSecret: {name: app-collection-auth, namespace: cattle-system}
---
apiVersion: catalog.cattle.io/v1
kind: ClusterRepo
metadata: {name: suse-registry}
spec:
  url: oci://registry.suse.com/ai/charts
  clientSecret: {name: suse-registry-auth, namespace: cattle-system}
---
apiVersion: catalog.cattle.io/v1
kind: ClusterRepo
metadata: {name: nvidia-charts}
spec:
  url: https://helm.ngc.nvidia.com/nvidia
  clientSecret: {name: ngc-helm-auth, namespace: cattle-system}
---
apiVersion: catalog.cattle.io/v1
kind: ClusterRepo
metadata: {name: nvidia-blueprint-charts}
spec:
  url: https://helm.ngc.nvidia.com/nvidia/blueprint
  clientSecret: {name: ngc-helm-auth, namespace: cattle-system}' \
  | kubectl --kubeconfig "$KCFG" apply -f -

# 3c. Wait for indexing — about 30s. Watch them flip to Downloaded=True
#     (or OCIDownloaded=True for the oci:// repos). nvidia-charts is
#     EXPECTED to fail with HTTP 403 because the test NGC API key is
#     scoped to /nvidia/blueprint only; ignore it for now.
kubectl --kubeconfig "$KCFG" get clusterrepos.catalog.cattle.io \
  -o custom-columns=NAME:.metadata.name,URL:.spec.url,DOWNLOAD:.status.downloadTime,MSG:.status.conditions[?\(@.reason==\"Error\"\)].message
```

**Recovery if a ClusterRepo gets stuck failing:** Rancher's catalog
controller backs off retries for 24h after a failure. `force-update`
annotation does NOT bypass this; only a generation bump does. To reset:

```fish
kubectl --kubeconfig "$KCFG" delete clusterrepo <name>
# then re-apply the manifest from 3b above
```

---

## 4. Create the operator's credential secrets and Settings CR

Settings holds references to these secrets — never the credentials
themselves. The CR name MUST be exactly `settings` (the operator hardcodes
this in `gitops.go:operatorSettingsName`).

```fish
# NVIDIA NGC creds
kubectl --kubeconfig "$KCFG" -n aif-operator create secret generic ngc-creds \
  --from-literal=username='$oauthtoken' \
  --from-literal=token='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'

# SUSE Registry creds
kubectl --kubeconfig "$KCFG" -n aif-operator create secret generic suse-registry-creds \
  --from-literal=username='regcode' \
  --from-literal=token='INTERNAL-USE-ONLY-c8d3-5629'

# SUSE App Collection creds
kubectl --kubeconfig "$KCFG" -n aif-operator create secret generic appco-creds \
  --from-literal=username='tbertoldi@suse.com' \
  --from-literal=token='Ynl1eXRya2VzdWRsaWJsZ2t6cm5sdGVybmNtcGt6Z3BpZmJieGlhdm9rY2t3dmlzcXB3Zm1vb2x4c2pkd2luZw=='

# Settings CR — name must be 'settings'
echo 'apiVersion: ai-platform.suse.com/v1alpha1
kind: Settings
metadata: {name: settings, namespace: aif-operator}
spec:
  nvidia:
    userSecretRef: {name: ngc-creds, key: username}
    tokenSecretRef: {name: ngc-creds, key: token}
  suseRegistry:
    userSecretRef: {name: suse-registry-creds, key: username}
    tokenSecretRef: {name: suse-registry-creds, key: token}
  applicationCollection:
    userSecretRef: {name: appco-creds, key: username}
    tokenSecretRef: {name: appco-creds, key: token}' \
  | kubectl --kubeconfig "$KCFG" apply -f -
```

---

## 5. Create a test Blueprint + AIWorkload that exercises NVIDIA injection

> **Naming note (changed in commit `9394013`):** the wizard now creates
> one AIWorkload CR per `(release, cluster)` pair, named
> `<release>-<clusterId>`. The user-visible release stays the
> short name. Throughout sections 5–8 below, `myai-$DOWNSTREAM`
> resolves to e.g. `myai-c-msmks` after `set` substitution.

If you installed with the `aiExtension.source.helm.values.image.*`
overrides in step 2, you have the branch's UI with the per-cluster CR
naming and the `vendor` field set correctly — you can build the
Blueprint via the UI wizard and skip 5a/5b entirely. The manifests below
are for a reproducible CLI-only test (or for triaging when the UI
extension is stuck on an older bundled version).

```fish
# 5a. Blueprint with vendor=nvidia. The actual chart may not fully deploy
#     (nvidia-blueprint-rag specifically needs ECK + NIM operators
#     pre-installed on downstream — see "Known gotchas"), but the
#     OPERATOR's pull-secret injection happens before Helm renders, so
#     section 6's verification ladder still works regardless.
echo 'apiVersion: ai-platform.suse.com/v1alpha1
kind: Blueprint
metadata:
  name: myai-1-0-0
  labels:
    ai-platform.suse.com/blueprint-name: myai
    ai-platform.suse.com/blueprint-version: 1.0.0
spec:
  displayName: myai
  version: 1.0.0
  components:
    - chartName: nvidia-blueprint-rag
      chartRepo: nvidia-blueprint-charts
      chartVersion: v2.6.0
      vendor: nvidia' \
  | kubectl --kubeconfig "$KCFG" apply -f -

# 5b. Target namespace + AIWorkload. CR name carries the cluster suffix
#     so a second install on a different cluster lives alongside this one
#     instead of colliding on (namespace, name).
kubectl --kubeconfig "$KCFG" create namespace myai-system --dry-run=client -o yaml \
  | kubectl --kubeconfig "$KCFG" apply -f -

echo "apiVersion: ai-platform.suse.com/v1alpha1
kind: AIWorkload
metadata:
  name: myai-$DOWNSTREAM
  namespace: myai-system
spec:
  displayName: myai
  source:
    sourceType: Blueprint
    blueprint: {name: myai, version: 1.0.0}
  targetNamespace: myai-system
  targetClusters: [$DOWNSTREAM]
  deployStrategy: FleetBundle" \
  | kubectl --kubeconfig "$KCFG" apply -f -
```

---

## 5.1 (Optional) Also exercise the App-source path

App-source workloads use a single chart (no Blueprint wrapper). The
operator now runs `reconcileAppPullSecrets` for these too, so the rest
of the delivery pipeline (consolidated Fleet Bundle, SA-merge Job,
`takeOwnership` on the workload HelmOp) is shared with the Blueprint
path.

```fish
kubectl --kubeconfig "$KCFG" create namespace myapp-system --dry-run=client -o yaml \
  | kubectl --kubeconfig "$KCFG" apply -f -

echo "apiVersion: ai-platform.suse.com/v1alpha1
kind: AIWorkload
metadata:
  name: myapp-$DOWNSTREAM
  namespace: myapp-system
spec:
  displayName: myapp
  source:
    sourceType: App
    app:
      chartRepo: nvidia-blueprint-charts
      chartName: aiq-aira
      chartVersion: v1.2.1
      release: myapp
      vendor: nvidia
  targetNamespace: myapp-system
  targetClusters: [$DOWNSTREAM]
  deployStrategy: FleetBundle" \
  | kubectl --kubeconfig "$KCFG" apply -f -
```

> Section 6's verification ladder works for `myapp-$DOWNSTREAM` /
> `myapp-system` by substitution.
>
> **Note:** the App+FleetBundle path's HelmOp is created by the **UI**,
> not the operator (`pkg/aif-ui/services/fleet-bundle.ts:upsertFleetHelmOp`).
> Section 5.1 above creates only the AIWorkload CR — the pull-secret
> delivery runs from that, but the actual chart install via Fleet HelmOp
> only fires when you submit through the UI wizard (which constructs the
> HelmOp from the wizard form). For a true CLI-only App flow you'd
> hand-craft the HelmOp YAML mirroring what the wizard produces.

---

## 6. Verify — the ladder, top to bottom

Each step should pass before moving to the next. All commands assume
`set DOWNSTREAM` from section 0 is in scope.

```fish
# 6.1. Did the operator inject? Expect names matching the workload's
#      vendor:
#        NVIDIA vendor    → ["ngc-secret","ngc-api"]
#        SUSE vendor      → ["suse-ai-pull-combined"]
#        Mixed Blueprint  → all three (one per component vendor)
kubectl --kubeconfig "$KCFG" -n myai-system get aiworkload myai-$DOWNSTREAM \
  -o jsonpath='{.status.pullSecretNames}{"\n"}'

# 6.2. Are the secrets on the local (mgmt) cluster?
kubectl --kubeconfig "$KCFG" -n myai-system get secrets ngc-secret ngc-api suse-ai-pull-combined --ignore-not-found

# 6.3. Did the operator emit the consolidated Fleet Bundle?
#      Expect ONE bundle: ai-pullsecrets-myai-$DOWNSTREAM-$DOWNSTREAM
#      (owner-name has the cluster suffix; the second segment is the target cluster).
kubectl --kubeconfig "$KCFG" -n fleet-default get bundles \
  -l "ai-platform.suse.com/owner-name=myai-$DOWNSTREAM" \
  -o custom-columns=NAME:.metadata.name,READY:.status.summary.ready,DESIRED:.status.summary.desiredReady,STATE:.status.display.state

# 6.4. Inspect the bundle's resources — expect (1 namespace) + (per-secret manifests)
#      + (SA-merge manifests). NVIDIA workload = 1 + 2 + 1 = 4; SUSE = 1 + 1 + 1 = 3.
kubectl --kubeconfig "$KCFG" -n fleet-default get bundle ai-pullsecrets-myai-$DOWNSTREAM-$DOWNSTREAM \
  -o jsonpath='{.spec.resources}' | jq 'length'

# 6.5. Did Fleet deliver everything to the downstream cluster?
#      Check both ngc-secret AND suse-ai-pull-combined; whichever the
#      operator decided to deliver should show up.
for s in ngc-secret ngc-api suse-ai-pull-combined; do
  echo "--- $s ---"
  kubectl --kubeconfig "$KCFG" get --raw \
    "/k8s/clusters/$DOWNSTREAM/api/v1/namespaces/myai-system/secrets/$s" 2>/dev/null \
    | jq '{type, name: .metadata.name, keys: (.data|keys)}'
end

# 6.6. Did the SA-merge Job run? Expect succeeded: 1. The completed Job is
#      intentionally retained: deleting it with ttlSecondsAfterFinished makes
#      Fleet report permanent drift and can trigger unsafe release cleanup in
#      Fleet versions whose implicit release-name handling disagrees at the
#      53-character Helm boundary.
kubectl --kubeconfig "$KCFG" get --raw \
  "/k8s/clusters/$DOWNSTREAM/apis/batch/v1/namespaces/myai-system/jobs" 2>/dev/null \
  | jq '.items[] | {name: .metadata.name, succeeded: .status.succeeded, failed: .status.failed}'

# 6.7. Did every SA in the namespace get imagePullSecrets?  This is the
#      ultimate proof the Job ran. The exact list depends on the vendor;
#      every SA in the namespace should reference at least one operator-
#      delivered secret name.
kubectl --kubeconfig "$KCFG" get --raw \
  "/k8s/clusters/$DOWNSTREAM/api/v1/namespaces/myai-system/serviceaccounts" 2>/dev/null \
  | jq '.items[] | {name: .metadata.name, imagePullSecrets}'
```

If a step fails:

| Failure | Likely cause | Fix |
|---|---|---|
| 6.1 returns empty/null | Operator pod is running old image; CRD is missing `pullSecretNames` field | `kubectl ... rollout restart deploy/aif-operator -n aif-operator`, then `kubectl apply -f charts/aif-operator/crds/` |
| 6.1 returns `["suse-ai-pull-combined"]` for an NVIDIA-vendor workload | Blueprint component (or App `spec.source.app.vendor`) is `"suse"` (the CRD default) | Patch the source: e.g. `kubectl ... patch blueprint myai-1-0-0 --type=json -p='[{"op":"replace","path":"/spec/components/0/vendor","value":"nvidia"}]'` then re-annotate the AIWorkload |
| 6.3 shows ErrApplied with "namespaces ... not found" | Operator image predates the Bundle-ships-namespace fix (`eaeffe2`) | Rebuild + push + rollout the operator |
| 6.3 shows "invalid ownership metadata" on a pull-secret bundle | Stale per-secret bundles from before the consolidated-bundle refactor (`2b4853e`) | Delete the stale bundle: `kubectl -n fleet-default delete bundle <old-name>` |
| 6.3 becomes Modified after about 10 minutes and Fleet agent logs `Deleting unknown bundle ID, helm uninstall` with a shortened actual release and longer `expectedRelease` | Operator image lacks the explicit 53-character Fleet-compatible `spec.helm.releaseName`; old images also delete the merge Job after 600 seconds, creating the first drift signal | Deploy the lifecycle-fixed operator. It emits Fleet's exact capped release name, retains the completed Job, and marks the Namespace `helm.sh/resource-policy: keep` so pull-secret release cleanup cannot delete the workload namespace |
| 6.7 shows the operator-delivered secret on the workload SA but the chart pod still ImagePullBackOffs | Pod was scheduled BEFORE the SA-merge Job ran | Delete the pod; its controller (Deployment/StatefulSet) recreates it and the new pod inherits the patched SA |
| Workload chart aborts with `Secret "ngc-secret" ... cannot be imported into the current release` | Operator/UI is missing the `takeOwnership=true` fix (`ee03c14`) | Rebuild operator + UI from `inject-nvidia-auth` HEAD; or set `imagePullSecret.create: false` and `ngcApiSecret.create: false` in the workload's `componentValues` as a per-chart workaround |
| NVIDIA workload pod ImagePullBackOff with 403 from nvcr.io, AND the in-cluster `ngc-secret` has labels `app.kubernetes.io/managed-by: Helm` / `helm.sh/chart: <nvidia-chart>` and decoded `auth = "$oauthtoken:"` (empty password) | Chart templated its own `ngc-secret` from empty `imagePullSecret.password` / `ngcApiSecret.password` values; `takeOwnership` let Helm adopt the operator's secret then overwrite the data | Operator image is missing the chart-secret-skip fix (`4518355`); rebuild operator + roll, then delete the broken AIWorkload and re-create from the UI so the chart re-installs with `imagePullSecret.create: false` / `ngcApiSecret.create: false` |
| Workload pod stuck Pending with `pod has unbound immediate PersistentVolumeClaims. not found` | Downstream cluster has no default StorageClass | Apply the `local-path` default-class patch from section 0 |
| Operator log: `bundles.fleet.cattle.io is forbidden` | ClusterRole `aif-operator` is the old one (no `bundles` CRUD) | `helm upgrade aif-operator charts/aif-operator/ --reuse-values` from the branch |

---

## 7. Iterate on the operator code

After editing operator source, push a fresh image and restart the pod:

```fish
cd /home/thbertoldi/suse/suse-ai-lifecycle-manager/aif-operator
set TAG "dev-"(date +%s)
docker build -t "ghcr.io/thbertoldi/aif-operator:$TAG" .
docker push "ghcr.io/thbertoldi/aif-operator:$TAG"
kubectl --kubeconfig "$KCFG" -n aif-operator set image \
  deploy/aif-operator manager="ghcr.io/thbertoldi/aif-operator:$TAG"
kubectl --kubeconfig "$KCFG" -n aif-operator rollout status deploy/aif-operator
cd /home/thbertoldi/suse/suse-ai-lifecycle-manager

# Force a fresh AIWorkload reconcile to exercise the new code
kubectl --kubeconfig "$KCFG" -n myai-system annotate aiworkload myai-$DOWNSTREAM \
  "force-reconcile="(date +%s) --overwrite
```

After editing UI source, rebuild + redeploy aif-ui:

```fish
cd /home/thbertoldi/suse/suse-ai-lifecycle-manager
yarn publish-pkgs -c -p -i '' -r ghcr.io -o thbertoldi -t aif-ui-0.1.0-dev.2
kubectl --kubeconfig "$KCFG" -n cattle-ui-plugin-system rollout restart deploy/aif-ui-server
# Hard-reload the Rancher UI (Ctrl+Shift+R) — the extension bundle is cached client-side.
```

---

## 8. Tear down (between test runs)

```fish
# AIWorkload + Blueprint
kubectl --kubeconfig "$KCFG" -n myai-system delete aiworkload myai-$DOWNSTREAM --ignore-not-found
kubectl --kubeconfig "$KCFG" delete blueprint myai-1-0-0 --ignore-not-found
kubectl --kubeconfig "$KCFG" delete namespace myai-system --ignore-not-found

# App-source workload (if you created 5.1)
kubectl --kubeconfig "$KCFG" -n myapp-system delete aiworkload myapp-$DOWNSTREAM --ignore-not-found
kubectl --kubeconfig "$KCFG" delete namespace myapp-system --ignore-not-found

# Downstream namespace + its content (via proxy)
kubectl --kubeconfig "$KCFG" delete --raw \
  "/k8s/clusters/$DOWNSTREAM/api/v1/namespaces/myai-system" 2>/dev/null

# Any leftover pull-secret bundles (the AIWorkload finalizer cleans these,
# but if delete was forced or the finalizer failed):
kubectl --kubeconfig "$KCFG" -n fleet-default delete bundle \
  -l "ai-platform.suse.com/owner-name=myai-$DOWNSTREAM" --ignore-not-found
```

Full teardown (uninstall operator):

```fish
helm uninstall aif-operator --kubeconfig "$KCFG" --namespace aif-operator
kubectl --kubeconfig "$KCFG" delete namespace aif-operator
# CRDs (helm install would have applied them; uninstall does NOT remove them):
kubectl --kubeconfig "$KCFG" delete -f charts/aif-operator/crds/
```

---

## Known gotchas

- **`nvidia-blueprint-rag` (v2.6.0) needs ECK + NIM operators
  pre-installed on the downstream cluster.** Helm renders ALL templates
  up-front and validates kinds against the cluster's RESTMapper, so the
  chart aborts with
  `no matches for kind "Elasticsearch" in version "elasticsearch.k8s.elastic.co/v1"`
  if ECK isn't there. Pull-secret delivery still completes (it runs
  before the chart render), so section 6 verification works regardless;
  the workload bundle itself just stays in `ErrApplied`. Most other
  NIM-family charts (e.g. `aiq-aira`, `k8s-nim-operator`) install
  cleanly with just the pull-secret delivery, thanks to commit
  `ee03c14`'s `takeOwnership=true`.
- **Default StorageClass on downstream.** Charts that use
  PersistentVolumeClaims (open-webui, milvus, ingestor-server, etc.)
  will stay Pending with
  `pod has unbound immediate PersistentVolumeClaims. not found` if the
  downstream cluster has no default StorageClass. See section 0 for the
  `local-path` default-class patch.
- **`nvidia-charts` ClusterRepo failing 403 is expected.** The test NGC
  API key is scoped to `/nvidia/blueprint` only. The UI's NVIDIA app
  list will show blueprint charts but not the full NIM catalog.
- **GPG timeout on `docker push`** — the docker credential helper
  (`pass`) occasionally wedges. Use the inline-auth workaround at the
  end of section 1.
- **Pull-secret Bundle release identity is explicit and migration-safe.**
  Bundle object names may be up to 63 characters, while Helm release names
  may be at most 53. Fleet v0.14.1 derives a shortened implicit release name
  but its release garbage collector compares it with the unshortened
  BundleDeployment name. The mismatch produces
  `Deleting unknown bundle ID, helm uninstall`. The operator therefore sets
  `spec.helm.releaseName` explicitly using Fleet's own MD5/5-character suffix
  algorithm. Do not replace it with a different capping algorithm during an
  upgrade: matching the already-installed release name prevents one final
  destructive uninstall. The shipped Namespace also carries
  `helm.sh/resource-policy: keep` as defense in depth.
- **One-shot SA-merge limitation.** The retained Job patches every SA in the
  namespace **at the time it runs**. SAs created later (by the deployed
  chart, for example) are not patched automatically because the completed
  Job's deterministic name remains unchanged. The
  operator partially mitigates this by injecting `imagePullSecrets`
  directly into chart values for NVIDIA components (so the chart-created
  SAs ship with the right pull-secret refs), and `takeOwnership=true`
  on the workload HelmOp lets the chart adopt the operator-delivered
  secrets so the chart's *own* SA template references survive the install.
  Treat a late chart-created SA without pull-secret references as a separate
  reconciliation gap; merely re-annotating the AIWorkload does not rerun an
  already-completed Job with unchanged inputs.
- **takeOwnership has an uninstall side-effect.** When the workload
  chart is uninstalled, Helm deletes the adopted pull-secrets along
  with the rest of the release. The pull-secret Bundle's Fleet reconcile
  recreates them within its next sync cycle, so it's a brief visibility
  gap, not data loss. A `helm.sh/resource-policy: keep` on the
  Bundle-side secrets would close the gap entirely — flagged as
  follow-up in commit `ee03c14`.
