# Runbook — evaluate downstream pull-secret delivery

End-to-end setup for testing the `inject-nvidia-auth` branch from a fresh
download of the Rancher kubeconfig. Designed to be copy-pasteable into a
**fish** shell, top to bottom. Assumes:

- Rancher management cluster reachable via downloaded kubeconfig.
- At least one downstream cluster registered with Rancher.
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

---

## 1. Make sure your operator image is built and pushed

If you already have `ghcr.io/thbertoldi/aif-operator:dev-<timestamp>` from
yesterday's session, you can skip this step. Otherwise:

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

---

## 2. Install the operator from the LOCAL branch chart (not the OCI one)

The published `oci://ghcr.io/suse/chart/aif-operator:0.1.0-dev.1` ships an
older CRD that strips `status.pullSecretNames` and an older ClusterRole
that lacks `bundles` CRUD. Install from the branch chart instead.

The `aiExtension.source.helm.values` block (added on this branch) pipes
through to the auto-installed aif-ui chart. Without it, you'd get the
upstream `ghcr.io/suse/aif-ui:0.1.0-dev.1` image — which lacks the
Blueprint wizard's `vendor` field, so the NVIDIA injector never fires
(see section 5 for the workaround in case you forget this override).

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
  --set aiExtension.source.helm.values.image.tag=0.1.0-dev.1 \
  --set aiExtension.source.helm.values.image.pullPolicy=Always

# Wait for rollout
kubectl --kubeconfig "$KCFG" -n aif-operator rollout status \
  deploy/aif-operator --timeout=120s

# The aif-ui Deployment is created asynchronously by the
# InstallAIExtension controller (~30s after operator pod is Ready).
# Confirm it picked up your image, not the upstream default:
kubectl --kubeconfig "$KCFG" -n cattle-ui-plugin-system get deploy aif-ui-server \
  -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
# Expected: ghcr.io/thbertoldi/aif-ui:0.1.0-dev.1
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

If you installed with the `aiExtension.source.helm.values.image.*`
overrides in step 2, you have the branch's UI with the `vendor` field
set correctly — you can build the Blueprint via the UI wizard normally
and skip 5a. If you forgot the overrides (or want to bypass the UI for
a reproducible test), apply the manifests below directly: the upstream
`0.1.0-dev.1` UI omits `vendor` from the request body, the CRD default
`"suse"` kicks in, the SUSE injector runs, and you never get
`ngc-secret`/`ngc-api`.

```fish
# 5a. Blueprint with vendor=nvidia. Stripped-down component values —
#     the actual chart won't deploy because the downstream cluster
#     doesn't have ECK operator, but the OPERATOR's injection happens
#     before Helm renders, so we still verify everything we care about.
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

# 5b. Target namespace + AIWorkload. Substitute $DOWNSTREAM with your
#     downstream cluster ID from step 0.
kubectl --kubeconfig "$KCFG" create namespace myai-system

echo "apiVersion: ai-platform.suse.com/v1alpha1
kind: AIWorkload
metadata:
  name: myai
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

## 6. Verify — the ladder, top to bottom

Each step should pass before moving to the next.

```fish
# 6.1. Did the operator inject? Expect ["ngc-secret","ngc-api"]
kubectl --kubeconfig "$KCFG" -n myai-system get aiworkload myai \
  -o jsonpath='{.status.pullSecretNames}{"\n"}'

# 6.2. Are the secrets on the local (mgmt) cluster?
kubectl --kubeconfig "$KCFG" -n myai-system get secret ngc-secret ngc-api

# 6.3. Did the operator emit the consolidated Fleet Bundle?
#      Expect ONE bundle: ai-pullsecrets-myai-$DOWNSTREAM
kubectl --kubeconfig "$KCFG" -n fleet-default get bundles \
  -l "ai-platform.suse.com/owner-name=myai" \
  -o custom-columns=NAME:.metadata.name,READY:.status.summary.ready,DESIRED:.status.summary.desiredReady,STATE:.status.display.state

# 6.4. Inspect the bundle's resources — expect 4 entries
#      (namespace + 2 secrets + SA-merge manifests)
kubectl --kubeconfig "$KCFG" -n fleet-default get bundle ai-pullsecrets-myai-$DOWNSTREAM \
  -o jsonpath='{.spec.resources}' | jq 'length'

# 6.5. Did Fleet deliver everything to the downstream cluster?
#      Expect ngc-secret + ngc-api with correct types and keys.
kubectl --kubeconfig "$KCFG" get --raw \
  "/k8s/clusters/$DOWNSTREAM/api/v1/namespaces/myai-system/secrets/ngc-secret" \
  | jq '{type, name: .metadata.name, keys: (.data|keys)}'
kubectl --kubeconfig "$KCFG" get --raw \
  "/k8s/clusters/$DOWNSTREAM/api/v1/namespaces/myai-system/secrets/ngc-api" \
  | jq '{type, name: .metadata.name, keys: (.data|keys)}'

# 6.6. Did the SA-merge Job run?  Expect succeeded: 1
kubectl --kubeconfig "$KCFG" get --raw \
  "/k8s/clusters/$DOWNSTREAM/apis/batch/v1/namespaces/myai-system/jobs" \
  | jq '.items[] | {name: .metadata.name, succeeded: .status.succeeded, failed: .status.failed}'

# 6.7. Did every SA in the namespace get imagePullSecrets?  Expect
#      [{name: "ngc-api"}, {name: "ngc-secret"}] on every SA in the ns.
kubectl --kubeconfig "$KCFG" get --raw \
  "/k8s/clusters/$DOWNSTREAM/api/v1/namespaces/myai-system/serviceaccounts" \
  | jq '.items[] | {name: .metadata.name, imagePullSecrets}'
```

If a step fails:

| Failure | Likely cause | Fix |
|---|---|---|
| 6.1 returns empty/null | Operator pod is running old image; CRD is missing `pullSecretNames` field | `kubectl ... rollout restart deploy/aif-operator -n aif-operator`, then `kubectl apply -f charts/aif-operator/crds/` |
| 6.1 returns `["suse-ai-pull-combined"]` only | Blueprint component has `vendor: suse` (default) | `kubectl ... patch blueprint myai-1-0-0 --type=json -p='[{"op":"replace","path":"/spec/components/0/vendor","value":"nvidia"}]'` then re-annotate the AIWorkload |
| 6.3 shows ErrApplied with "namespaces ... not found" | Operator image predates the Bundle-ships-namespace fix | Rebuild + push + rollout the operator |
| 6.3 shows ErrApplied with "invalid ownership metadata" | Stale namespace from prior per-secret bundles | `kubectl ... delete namespace myai-system --context=$DOWNSTREAM` (via raw proxy) and reapply, OR confirm operator has `takeOwnership=true` in the Bundle spec |
| 6.6 shows `failed: 1` and Job pod is gone | Container image lacks `kubectl` or script failed | Get pod logs via the proxy: `kubectl --kubeconfig "$KCFG" get --raw "/k8s/clusters/$DOWNSTREAM/api/v1/namespaces/myai-system/pods/<pod>/log"` |
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
kubectl --kubeconfig "$KCFG" -n myai-system annotate aiworkload myai \
  "force-reconcile="(date +%s) --overwrite
```

---

## 8. Tear down (between test runs)

```fish
# AIWorkload + Blueprint
kubectl --kubeconfig "$KCFG" -n myai-system delete aiworkload myai --ignore-not-found
kubectl --kubeconfig "$KCFG" delete blueprint myai-1-0-0 --ignore-not-found
kubectl --kubeconfig "$KCFG" delete namespace myai-system --ignore-not-found

# Downstream namespace + its content (via proxy)
kubectl --kubeconfig "$KCFG" delete --raw \
  "/k8s/clusters/$DOWNSTREAM/api/v1/namespaces/myai-system" 2>/dev/null

# Any leftover pullsecret bundles (the AIWorkload finalizer should clean
# these, but if delete was forced or finalizer failed):
kubectl --kubeconfig "$KCFG" -n fleet-default delete bundle \
  -l "ai-platform.suse.com/owner-name=myai" --ignore-not-found
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

- **Don't expect the workload to deploy.** The `nvidia-blueprint-rag`
  chart needs the ECK operator + several NVIDIA operators pre-installed
  on the downstream cluster. We're only verifying the **operator's
  pull-secret machinery**, not the chart itself. The workload Bundle
  (`myai-nvidia-blueprint-rag`) will stay in `ErrApplied` with
  `no matches for kind "Elasticsearch"` — that's expected and irrelevant
  to this evaluation.
- **`nvidia-charts` ClusterRepo failing 403 is expected.** The test NGC
  API key is scoped to `/nvidia/blueprint` only. The UI's NVIDIA app
  list will show blueprint charts but not the full NIM catalog.
- **GPG timeout on `docker push`**: the docker credential helper
  occasionally times out (`gpg: decryption failed: Timeout`). Retry the
  command; it's transient.
- **One-shot SA-merge limitation:** the Job patches every SA in the
  namespace **at the time it runs**. SAs created later (by the deployed
  chart, for example) won't be patched until the Bundle re-applies. For
  charts that create their own SAs, either rely on `values.imagePullSecrets`
  injection (which the operator already does for NVIDIA charts) or
  re-trigger the AIWorkload reconcile after the chart deploys.
