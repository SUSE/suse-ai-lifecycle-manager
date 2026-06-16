package cluster

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// bundleGVK is the Fleet Bundle CR GVK we emit.
var bundleGVK = schema.GroupVersionKind{Group: "fleet.cattle.io", Version: "v1alpha1", Kind: "Bundle"}

// BundleClientOptions configures a bundleClient instance.
type BundleClientOptions struct {
	// ClusterID is the Rancher cluster ID Fleet targets (e.g. "c-abc123").
	// Use "local" for the operator's own cluster (the bundleClient would
	// then deliver via fleet-local rather than fleet-default).
	ClusterID string
	// FleetWorkspace is the Fleet workspace namespace the Bundle lives in.
	// Conventionally "fleet-default" for downstream clusters, "fleet-local"
	// for the operator's own cluster.
	FleetWorkspace string
	// OwnerName is the AIWorkload name owning this Bundle. Used in Bundle
	// naming and labels so the finalizer can delete it by label selector.
	OwnerName string
	// OwnerNamespace is the AIWorkload namespace owning this Bundle.
	OwnerNamespace string
}

// NewBundleClient returns a Client that emits one Fleet Bundle per
// ApplySecret call (one Bundle per Secret, per target cluster). Bundle
// names follow "ai-pullsecrets-<OwnerName>-<ClusterID>-<SecretName>" so
// multiple secrets for the same owner+cluster don't collide. Calls are
// idempotent: re-applying the same Secret updates the same Bundle in
// place. Owner labels (ai-platform.suse.com/owner-name and
// /owner-namespace) tie all of an AIWorkload's bundles together for the
// finalizer to clean up via label selector.
func NewBundleClient(c client.Client, scheme *runtime.Scheme, opts BundleClientOptions) Client {
	return &bundleClient{c: c, scheme: scheme, opts: opts}
}

type bundleClient struct {
	c client.Client
	// scheme kept for symmetry with localClient; not currently used.
	scheme *runtime.Scheme
	opts   BundleClientOptions
}

func (b *bundleClient) ApplySecret(ctx context.Context, sec *corev1.Secret) error {
	// Defensive copy + ensure TypeMeta so the serialized form is self-contained
	// (Fleet just applies the YAML as-is on the target cluster).
	out := sec.DeepCopy()
	out.APIVersion = "v1"
	out.Kind = "Secret"

	secYAML, err := yaml.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshal secret %s/%s: %w", out.Namespace, out.Name, err)
	}

	// Fleet wraps each Bundle as its own Helm release on the target cluster,
	// so a Secret targeted at a not-yet-existing namespace fails with
	// "namespaces \"X\" not found". Ship a Namespace manifest alongside the
	// Secret so the target namespace is guaranteed to exist before the Secret
	// applies.
	//
	// Because multiple bundles for the same (owner, cluster) each ship the
	// same Namespace, the second bundle's Helm release would otherwise refuse
	// to adopt a namespace already annotated as owned by the first release.
	// The Bundle below sets spec.helm.takeOwnership=true so Helm overrides
	// the adoption check, and the Namespace gets helm.sh/resource-policy=keep
	// so deleting one bundle doesn't drop the namespace out from under any
	// peer bundle still referencing it.
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        out.Namespace,
			Annotations: map[string]string{"helm.sh/resource-policy": "keep"},
		},
	}
	nsYAML, err := yaml.Marshal(ns)
	if err != nil {
		return fmt.Errorf("marshal namespace %s: %w", out.Namespace, err)
	}

	bundleName := fmt.Sprintf("ai-pullsecrets-%s-%s-%s", b.opts.OwnerName, b.opts.ClusterID, sec.Name)
	nsResourceName := fmt.Sprintf("%s-namespace.yaml", out.Namespace)
	resourceName := fmt.Sprintf("%s-%s.yaml", out.Namespace, out.Name)

	bundle := &unstructured.Unstructured{}
	bundle.SetGroupVersionKind(bundleGVK)
	bundle.SetName(bundleName)
	bundle.SetNamespace(b.opts.FleetWorkspace)
	bundle.SetLabels(map[string]string{
		"ai-platform.suse.com/owner-name":      b.opts.OwnerName,
		"ai-platform.suse.com/owner-namespace": b.opts.OwnerNamespace,
	})

	spec := map[string]any{
		"resources": []any{
			map[string]any{
				"name":    nsResourceName,
				"content": string(nsYAML),
			},
			map[string]any{
				"name":    resourceName,
				"content": string(secYAML),
			},
		},
		"targets": []any{
			map[string]any{
				"clusterName": b.opts.ClusterID,
			},
		},
		// takeOwnership: peer pull-secret bundles for the same (owner, cluster)
		// all carry the same Namespace manifest; without this, the second Helm
		// release refuses to adopt the namespace the first release annotated.
		"helm": map[string]any{
			"takeOwnership": true,
		},
	}
	if err := unstructured.SetNestedField(bundle.Object, spec, "spec"); err != nil {
		return fmt.Errorf("set bundle spec: %w", err)
	}

	if err := b.c.Patch(ctx, bundle, client.Apply, client.ForceOwnership, client.FieldOwner("suse-ai-operator")); err != nil {
		return fmt.Errorf("apply bundle %s/%s: %w", bundle.GetNamespace(), bundle.GetName(), err)
	}
	return nil
}
