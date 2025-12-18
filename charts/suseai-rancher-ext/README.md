# SUSE AI Rancher UI Extension

This chart installs the **Rancher Extension Catalog**, it contains extension assets bundled into an image and act as a catalog for custom extensions.
1. This Chart is installed.
2. The catalog needs to be added as a Helm repository in the Rancher Repositories. 
3. The extension packaged as a container image is registered in Rancher via the UIPlugin custom resource.

**Homepage:** <https://github.com/SUSE/suse-ai-rancher-ext>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| SUSE LLC |  | <https://www.suse.com> |

## Prerequisites

- The following CRDs must exist:
    - `uiplugins.catalog.cattle.io`
    - `clusterrepos.catalog.cattle.io`

You can verify with:
```bash
kubectl get crd uiplugins.catalog.cattle.io
```

## Installing the Chart

This chart is distributed as an OCI Helm chart. To install the chart with the release name `suse-ui-ext`:

```bash
helm install suse-ui-ext \
  -n cattle-ui-plugin-system \
  oci://ghcr.io/suse/chart/suse-ai-rancher-ext
```

The command deploys the suse ai extension catalog on the Kubernetes cluster in the default configuration. The [Parameters](#parameters) section lists the parameters that can be configured during installation.

## Uninstalling the Chart

To uninstall/delete the `suse-ui-ext` deployment:

```bash
helm uninstall suse-ui-ext -n cattle-ui-plugin-system
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Parameters

### Global parameters

| Name                | Description                                   | Value |
| ------------------- | --------------------------------------------- | ----- |
| `replicaCount`      | Number of suse-ai-rancher-ext replicas to deploy             | `1`   |
| `nameOverride`      | String to partially override suse-ai-rancher-ext.fullname    | `""`  |
| `fullnameOverride`  | String to fully override suse-ai-rancher-ext.fullname        | `""`  |

### Image parameters

| Name               | Description                                | Value                        |
| ------------------ | ------------------------------------------ | ---------------------------- |
| `image.registry`   | suse-ai-rancher-ext image registry                        | `ghcr.io`         |
| `image.repository` | suse-ai-rancher-ext image repository                      | `suse/suseai-rancher-ext` |
| `image.tag`        | suse-ai-rancher-ext image tag (immutable tags recommended)| `0.1.0`                     |
| `image.pullPolicy` | suse-ai-rancher-ext image pull policy                     | `IfNotPresent`               |
| `imagePullSecrets` | suse-ai-rancher-ext image pull secrets                    | `[]`                         |
| `global.imagePullSecrets` | Global override for container image registry pull secrets |`[]`    |
| `global.imageRegistry` | Global override for container image registry | `""`                   |

### Service parameters

| Name           | Description             | Value       |
| -------------- | ----------------------- | ----------- |
| `service.type` | suse-ai-rancher-ext service type       | `ClusterIP` |
| `service.port` | suse-ai-rancher-ext service HTTP port  | `8080`      |


## Troubleshooting

### Check pod status

```bash
kubectl get pods -l app.kubernetes.io/name=suse-ai-rancher-ext
```

### Check logs

```bash
kubectl logs -l app.kubernetes.io/name=suse-ai-rancher-ext
```
