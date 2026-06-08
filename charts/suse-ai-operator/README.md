# SUSE AI Operator

Helm chart to deploy the SUSE AI Operator on Kubernetes.

The SUSE AI Operator manages the lifecycle of AI extensions in a Rancher-managed cluster using the `InstallAIExtension` custom resource.
It supports both Helm charts and Git repositories as extension sources, and integrates with Rancher catalogs (ClusterRepo) and UI plugins (UIPlugin) to enable declarative installation and management. For git sources, the operator supports managed and unmanaged version policies to control whether the operator or the user manages extension upgrades.


**Homepage:** <https://github.com/SUSE/suse-ai-lifecycle-manager/suse-ai-operator>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| SUSE LLC |  | <https://www.suse.com> |

## Prerequisites

- Kubernetes 1.24+
- Helm 3.x
- Rancher installed (for UIPlugin and ClusterRepo integration)
- cert-manager installed (required when `webhook.enable=true` and `webhook.certManager.enable=true`)

The following CRDs must exist before adding the operator:
  - `uiplugins.catalog.cattle.io`
  - `clusterrepos.catalog.cattle.io`

You can verify with:
```bash
kubectl get crd uiplugins.catalog.cattle.io
kubectl get crd clusterrepos.catalog.cattle.io
```

## CRD Management

This chart ships CRDs as Helm templates (in `templates/crds/`) rather than the standard `crds/` directory. This is required because the CRD includes conditional conversion webhook configuration that depends on chart values.

**How It Works**
- CRDs are installed and **upgraded** automatically by Helm (unlike standard `crds/` behavior)
- CRDs are **not deleted** on `helm uninstall` (protected by `"helm.sh/resource-policy": keep` annotation)
- The CRD includes conversion webhook configuration when `webhook.enable=true`
- cert-manager injects the CA bundle when `webhook.certManager.enable=true`

**Manual CRD Deletion**
After uninstalling the chart, remove the CRD manually if desired:

`kubectl delete crd installaiextensions.ai-platform.suse.com`

## Installing the Chart

This chart is distributed as an OCI Helm chart. Install the chart with the release name `suse-ai-operator`:

```bash
helm install suse-ai-operator \
  -n suse-ai-operator-system \
  --create-namespace \
  oci://ghcr.io/suse/chart/suse-ai-operator
```

By default, the chart also creates an `InstallAIExtension` CR to install the SUSE AI Lifecycle Manager extension. This is controlled by `extension.enable` (default: `true`). The CR is created as a `post-install` hook, ensuring the operator is deployed before the extension is applied.

To install the operator without the bundled extension:
```bash
helm install suse-ai-operator \
  -n suse-ai-operator-system \
  --create-namespace \
  --set extension.enable=false \
  oci://ghcr.io/suse/chart/suse-ai-operator
```

The command deploys the SUSE AI Operator using the default configuration. See the [Parameters](#parameters) section for configurable options.

## Uninstalling the Chart

To uninstall the operator:

```bash
helm uninstall suse-ai-operator -n suse-ai-operator-system
```

When `extension.enable=true`, a `pre-delete` cleanup Job automatically deletes the `InstallAIExtension` CR before the operator is removed. This ensures the operator's finalizer can properly clean up Helm releases, ClusterRepos, and UIPlugins while the operator is still running.

This removes all Kubernetes resources created by the chart **except CRDs**, which must be removed manually if desired:

`kubectl delete crd installaiextensions.ai-platform.suse.com`

## Parameters

### Global parameters

| Name                      | Description                        | Value |
| ------------------------- | ---------------------------------- | ----- |
| `global.imageRegistry`    | Global override for image registry | `""`  |
| `global.imagePullSecrets` | Global image pull secrets          | `[]`  |
| `nameOverride`            | Partially override chart name      | `""`  |
| `fullnameOverride`        | Fully override resource names      | `""`  |

### Manager parameters

#### General

| Name                       | Description                       | Default              |
| -------------------------- | --------------------------------- | -------------------- |
| `manager.replicaCount`     | Number of operator replicas       | `1`                  |
| `manager.args`             | Additional command-line arguments | `["--leader-elect"]` |
| `manager.env`              | Extra environment variables       | `[]`                 |
| `manager.imagePullSecrets` | Image pull secrets                | `[]`                 |
| `manager.podAnnotations`   | Pod annotations                   | `{}`                 |

#### Image

| Name                       | Description               | Default                 |
| -------------------------- | ------------------------- | ----------------------- |
| `manager.image.registry`   | Operator image registry   | `ghcr.io`               |
| `manager.image.repository` | Operator image repository | `suse/suse-ai-operator` |
| `manager.image.tag`        | Operator image tag        | `""`                    |
| `manager.image.pullPolicy` | Image pull policy         | `IfNotPresent`          |

#### Pod Security Context

| Name                                             | Description               | Default          |
| ------------------------------------------------ | ------------------------- | ---------------- |
| `manager.podSecurityContext.runAsNonRoot`        | Run container as non-root | `true`           |
| `manager.podSecurityContext.seccompProfile.type` | Seccomp profile type      | `RuntimeDefault` |

#### Container Security Context

| Name                                               | Description                | Default   |
| -------------------------------------------------- | -------------------------- | --------- |
| `manager.securityContext.allowPrivilegeEscalation` | Allow privilege escalation | `false`   |
| `manager.securityContext.readOnlyRootFilesystem`   | Read-only root filesystem  | `true`    |
| `manager.securityContext.capabilities.drop`        | Linux capabilities to drop | `["ALL"]` |

#### Resources

| Name                                | Description    | Default |
| ----------------------------------- | -------------- | ------- |
| `manager.resources.requests.cpu`    | CPU request    | `10m`   |
| `manager.resources.requests.memory` | Memory request | `64Mi`  |
| `manager.resources.limits.cpu`      | CPU limit      | `500m`  |
| `manager.resources.limits.memory`   | Memory limit   | `128Mi` |

#### Probes

| Name                                          | Description           | Default    |
| --------------------------------------------- | --------------------- | ---------- |
| `manager.probes.liveness.enabled`             | Enable liveness probe | `true`     |
| `manager.probes.liveness.httpGet.path`        | Liveness probe path   | `/healthz` |
| `manager.probes.liveness.httpGet.port`        | Liveness probe port   | `8081`     |
| `manager.probes.liveness.periodSeconds`       | Probe period          | `20`       |
| `manager.probes.liveness.initialDelaySeconds` | Initial delay         | `15`       |
| `manager.probes.readiness.enabled`             | Enable readiness probe | `true`    |
| `manager.probes.readiness.httpGet.path`        | Readiness probe path   | `/readyz` |
| `manager.probes.readiness.httpGet.port`        | Readiness probe port   | `8081`    |
| `manager.probes.readiness.periodSeconds`       | Probe period           | `10`      |
| `manager.probes.readiness.initialDelaySeconds` | Initial delay          | `5`       |

#### Scheduling

| Name                   | Description        | Default |
| ---------------------- | ------------------ | ------- |
| `manager.nodeSelector` | Node selector      | `{}`    |
| `manager.tolerations`  | Pod tolerations    | `[]`    |
| `manager.affinity`     | Pod affinity rules | `{}`    |

### Metrics parameters

| Name             | Description             | Default |
| ---------------- | ----------------------- | ------- |
| `metrics.enable` | Enable metrics endpoint | `true`  |
| `metrics.port`   | Metrics HTTPS port      | `8443`  |

> When enabled, a metrics Service and RBAC rules are created to support authenticated scraping.

### RBAC helper roles 

| Name                 | Description                                      | Default |
| -------------------- | ------------------------------------------------ | ------- |
| `rbacHelpers.enable` | Create helper ClusterRoles (admin/editor/viewer) | `false` |

### Extension parameters

| Name                     | Description                                  | Default  |
|--------------------------|----------------------------------------------|----------|
| `extension.enable`       | Create an InstallAIExtension CR with the chart | `true` |
| `extension.crName`       | Name of the InstallAIExtension               | `suseai` |
| `extension.chartVersion` | Helm chart version for the extension         | `1.1.0`  |
| `extension.version`      | Extension version (`spec.extension.version`) | `1.1.0`  |

> When enabled, the CR is created as a `post-install`/`post-upgrade` hook to ensure the operator is ready. On `helm uninstall`, a `pre-delete` cleanup Job deletes the CR first so the operator can run its finalizer before being removed.

### Webhook parameters

| Name                          | Description                                    | Default |
| ----------------------------- | ---------------------------------------------- | ------- |
| `webhook.enable`              | Enable conversion webhook (v1alpha1 <-> v1beta1) | `true`  |
| `webhook.port`                | Webhook server port                            | `9443`  |
| `webhook.certManager.enable`  | Use cert-manager for webhook TLS certificates  | `true`  |
| `webhook.certManager.issuerRef` | Optional: use a specific Issuer/ClusterIssuer | `{}`    |

> When enabled, a Service, Certificate, and Issuer are created. The CRD is configured with a conversion webhook pointing to the operator's `/convert` endpoint.

## Troubleshooting

### Check pod status

```bash
kubectl get pods -l app.kubernetes.io/name=suse-ai-operator -n suse-ai-operator-system
```

### Check logs

```bash
kubectl logs deploy/suse-ai-operator -n suse-ai-operator-system -f
```

### Metrics endpoint not reachable

* Ensure `metrics.enable=true`
* Verify the metrics Service exists:
``` bash
kubectl get svc -n suse-ai-operator-system
```
* Confirm RBAC permissions allow access to `/metrics`

### CRD not found errors

* Ensure the CRD exists:
``` bash
kubectl get crd installaiextensions.ai-platform.suse.com
```
* Re-apply CRDs manually if required

### Conversion webhook errors

* Ensure cert-manager is installed and the Certificate is ready:
```bash
kubectl get certificate -n suse-ai-operator-system
```

* Check the webhook service exists:
```bash
kubectl get svc -l app.kubernetes.io/name=suse-ai-operator -n suse-ai-operator-system
```

* If not using cert-manager, disable the webhook: --set webhook.enable=false

### Extension stuck in Installing phase

* The operator waits up to 5 minutes for the Helm deployment to become ready
* Check the deployment status:
```bash
kubectl get deployments -n suse-ai-operator-system
```
* After 5 minutes the extension status changes to Failed with a timeout message
