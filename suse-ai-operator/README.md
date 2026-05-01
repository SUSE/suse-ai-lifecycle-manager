# SUSE AI Operator
The SUSE AI Extension Operator installs and manages Rancher UI extension for SUSE AI using a declarative Kubernetes Custom Resource (CR). It acts as a bridge between Helm-based extension packaging and Rancher UIPlugin resources, handling lifecycle, validation, retries, and cleanup in a Kubernetes-native way.

## Purpose
This operator exists to:
- Install SUSE AI Rancher UI extensions safely and declaratively.
- Support both Helm charts and Git repositories as extension sources.
- Support managed and unmanaged version policies for git sources.
- Prevent conflicts with operator-unmanaged Helm resources.
- Manage Helm releases, ClusterRepos, and UIPlugins.
- Detect source type changes and clean up stale resources automatically.

## Getting Started

### Prerequisites
- go v1.24.0+
- docker v17.03+
- kubectl v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster
- Helm 3.x
- Rancher installed (for UIPlugin and ClusterRepo integration)
- cert-manager installed (for conversion webhook TLS certificates)

The following CRDs must exist before adding the operator:
  - `uiplugins.catalog.cattle.io`
  - `clusterrepos.catalog.cattle.io`

### Installation

The operator is distributed as a Helm chart and installs:
- Controller Deployment
- Conversion Webhook (v1alpha1 <-> v1beta1)
- Default InstallAIExtension CR (configurable, enabled by default)
- Pre-delete cleanup Job (ensures clean uninstall)
- RBAC
- CRDs
- Metrics Service

1. **Install the SUSE AI Operator.** First, install the operator via Helm:

```sh
helm install suse-ai-operator \
  -n suse-ai-operator-system \
  --create-namespace \
  oci://ghcr.io/suse/chart/suse-ai-operator
```

This will deploy the SUSE AI Operator into the `suse-ai-operator-system` namespace.

The operator deploys a conversion webhook for API version compatibility (v1alpha1 <-> v1beta1). This requires cert-manager to be installed in the cluster for automatic TLS certificate management. To disable the webhook (single-version mode), set `--set webhook.enable=false`.

By default, the chart also creates an `InstallAIExtension` CR to install the SUSE AI Lifecycle Manager extension. To install the operator without the bundled extension, set --set `extension.enable=false`.

2. **(Optional) Create the InstallAIExtension CR.** By default, the chart already creates an InstallAIExtension CR that installs the SUSE AI Lifecycle Manager extension. Skip this step if the default configuration is sufficient.

If you need a custom configuration, or installed with `--set extension.enable=false`, create your own CR:
**Using a Helm source:**
```yaml
apiVersion: ai-platform.suse.com/v1beta1
kind: InstallAIExtension
metadata:
  name: suseai
spec:
  source:
    helm:
      name: suse-ai-lifecycle-manager
      url: "oci://ghcr.io/suse/chart/suse-ai-lifecycle-manager"
      version: "1.1.0"
  extension:
    name: suse-ai-lifecycle-manager
    version: "1.1.0"
```

**Or Using a Git source (managed, pinned version):**
```yaml
apiVersion: ai-platform.suse.com/v1beta1
kind: InstallAIExtension
metadata:
  name: suseai
spec:
  source:
    git:
      repo: https://github.com/SUSE/suse-ai-lifecycle-manager
      branch: gh-pages
  extension:
    name: suse-ai-lifecycle-manager
    version: "1.1.0"
```

**Or Using a Git source (managed, latest version):**
```yaml
apiVersion: ai-platform.suse.com/v1beta1
kind: InstallAIExtension
metadata:
  name: suseai
spec:
  source:
    git:
      repo: https://github.com/SUSE/suse-ai-lifecycle-manager
      branch: gh-pages
  extension:
    name: suse-ai-lifecycle-manager
```

**Or Using a Git source (unmanaged — user controls upgrades via Rancher UI):**
```yaml
apiVersion: ai-platform.suse.com/v1beta1
kind: InstallAIExtension
metadata:
  name: suseai
spec:
  source:
    git:
      repo: https://github.com/SUSE/suse-ai-lifecycle-manager
      branch: gh-pages
  extension:
    name: suse-ai-lifecycle-manager
    version: "1.1.0"
    versionPolicy: unmanaged
```

Apply this file
```sh
kubectl apply -f extension.yaml
```

> **NOTE:** The v1alpha1 API version with `spec.helm` is still supported but deprecated. Use `spec.source.helm` or `spec.source.git` in v1beta1 instead.

> **NOTE:** Each `spec.extension.name` must be unique across all InstallAIExtension resources. The operator will reject duplicates with a `Failed` status.

> **NOTE:** The `versionPolicy` field controls how the operator manages the UIPlugin for git sources:
> - `managed` (default): The operator fully controls the UIPlugin. If `version` is set, it pins to that version. If `version` is omitted, it automatically resolves and tracks the latest version. Users cannot modify or uninstall the extension from the Rancher UI.
> - `unmanaged`: The operator installs the UIPlugin once, then hands off. Users can upgrade, downgrade, or uninstall the extension from Rancher's Extensions page.
>
> Helm sources always behave as managed with an explicit version — `versionPolicy` is not applicable.


### Uninstall

To uninstall the operator:
```sh
helm uninstall suse-ai-operator -n suse-ai-operator-system
```

When the bundled extension is enabled (`extension.enable=true`), a pre-delete cleanup Job automatically deletes the `InstallAIExtension` CR and waits for the operator to finish cleaning up Helm releases, ClusterRepos, and UIPlugins before the operator is removed.

If you installed the extension manually (with `extension.enable=false`), delete the CR first:
```sh
kubectl delete iae suseai
helm uninstall suse-ai-operator -n suse-ai-operator-system
```

After uninstalling, remove the CRD if desired:
```sh
kubectl delete crd installaiextensions.ai-platform.suse.com
```

## Development

### To Build and Test locally
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/suse-ai-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs and Deploy the Operator into the cluster:**

```sh
helm install suse-ai-operator ./charts/suse-ai-operator/ -n suse-ai-operator-system \
    --create-namespace \
    --set manager.image.registry=<some-registry> \ 
    --set manager.image.repository=<repository-owner>/suse-ai-operator \
    --set manager.image.tag=<tag>
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create CRs**
You can apply the sample (example) from the samples/:

>**NOTE**: Don't apply all the samples, choose one.

```sh
kubectl apply -k samples/
```

>**NOTE**: Ensure that the sample has default values to test it out. 

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k samples/
```

**UnDeploy the controller from the cluster:**

```sh
helm uninstall suse-ai-operator -n suse-ai-operator-system
```

**Delete the APIs(CRDs) from the cluster:**

```sh
kubectl delete crd installaiextensions.ai-platform.suse.com
```

## Testing

1. **Install Rancher (or mock CRDs)**

2. **Install the operator:**

```bash
helm install suse-ai-operator ./charts/suse-ai-operator -n suse-ai-operator-system \
    --create-namespace
```

3. **Apply an extension:**
```bash
kubectl apply -f samples/installaiextension.yaml
```

4. **Observe reconciliation:**
```bash
kubectl logs -l app.kubernetes.io/name=suse-ai-operator -f -n suse-ai-operator-system
```

5. **Verify resources:**
```bash
kubectl get iae
kubectl get uiplugins -A
kubectl get clusterrepos
helm list -A
```

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

