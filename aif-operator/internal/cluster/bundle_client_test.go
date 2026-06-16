package cluster_test

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/SUSE/aif-operator/internal/cluster"
)

var bundleGVK = schema.GroupVersionKind{Group: "fleet.cattle.io", Version: "v1alpha1", Kind: "Bundle"}
var bundleListGVK = schema.GroupVersionKind{Group: "fleet.cattle.io", Version: "v1alpha1", Kind: "BundleList"}

func TestBundleClient_EmitsBundleCarryingSecret(t *testing.T) {
	scheme := newBundleTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	bc := cluster.NewBundleClient(c, scheme, cluster.BundleClientOptions{
		ClusterID:      "c-abc123",
		FleetWorkspace: "fleet-default",
		OwnerName:      "wl-x",
		OwnerNamespace: "default",
	})

	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ngc-secret", Namespace: "target-ns"},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte(`{"auths":{}}`)},
	}
	if err := bc.ApplySecret(context.Background(), sec); err != nil {
		t.Fatalf("ApplySecret: %v", err)
	}

	bundleName := "ai-pullsecrets-wl-x-c-abc123-ngc-secret"
	var bundle unstructured.Unstructured
	bundle.SetGroupVersionKind(bundleGVK)
	if err := c.Get(context.Background(),
		client.ObjectKey{Namespace: "fleet-default", Name: bundleName}, &bundle); err != nil {
		t.Fatalf("get bundle %s/%s: %v", "fleet-default", bundleName, err)
	}

	// Owner labels are set
	labels := bundle.GetLabels()
	if labels["ai-platform.suse.com/owner-name"] != "wl-x" {
		t.Errorf("owner-name label: got %q want wl-x", labels["ai-platform.suse.com/owner-name"])
	}
	if labels["ai-platform.suse.com/owner-namespace"] != "default" {
		t.Errorf("owner-namespace label: got %q want default", labels["ai-platform.suse.com/owner-namespace"])
	}

	// targets[0].clusterName is the cluster ID
	targets, found, err := unstructured.NestedSlice(bundle.Object, "spec", "targets")
	if err != nil || !found || len(targets) == 0 {
		t.Fatalf("spec.targets missing: found=%v err=%v", found, err)
	}
	t0, _ := targets[0].(map[string]any)
	if name, _ := t0["clusterName"].(string); name != "c-abc123" {
		t.Errorf("target clusterName: got %v want c-abc123", name)
	}

	// resources holds [0] the target Namespace, [1] the Secret. The Namespace
	// is shipped because Fleet applies bundle resources verbatim with no
	// namespace creation; without it a Secret targeted at a not-yet-existing
	// namespace fails with "namespaces \"X\" not found".
	resources, found, err := unstructured.NestedSlice(bundle.Object, "spec", "resources")
	if err != nil || !found || len(resources) != 2 {
		t.Fatalf("spec.resources missing or wrong count: found=%v err=%v len=%d (want 2: namespace + secret)", found, err, len(resources))
	}

	// resources[0] is the Namespace manifest, annotated with
	// helm.sh/resource-policy=keep so peer bundles sharing this namespace
	// don't delete it when one is removed.
	r0, _ := resources[0].(map[string]any)
	nsContent, _ := r0["content"].(string)
	if !strings.Contains(nsContent, "kind: Namespace") || !strings.Contains(nsContent, "name: target-ns") {
		t.Errorf("resources[0] should be a Namespace for target-ns, got:\n%s", nsContent)
	}
	if !strings.Contains(nsContent, "helm.sh/resource-policy: keep") {
		t.Errorf("namespace manifest missing helm.sh/resource-policy=keep annotation, got:\n%s", nsContent)
	}
	if name, _ := r0["name"].(string); name == "" {
		t.Errorf("resources[0].name is empty")
	}

	// spec.helm.takeOwnership must be true so peer pull-secret bundles can
	// share the namespace without Helm's adoption check rejecting the second.
	takeOwnership, found, err := unstructured.NestedBool(bundle.Object, "spec", "helm", "takeOwnership")
	if err != nil || !found || !takeOwnership {
		t.Errorf("spec.helm.takeOwnership: found=%v err=%v value=%v (want true)", found, err, takeOwnership)
	}

	// resources[1] is the serialized Secret
	r1, _ := resources[1].(map[string]any)
	content, _ := r1["content"].(string)
	if content == "" {
		t.Errorf("resources[1].content is empty")
	}
	if !strings.Contains(content, "ngc-secret") || !strings.Contains(content, "kubernetes.io/dockerconfigjson") {
		t.Errorf("serialized resource content missing expected fields:\n%s", content)
	}
	// resource name should namespace-disambiguate so multiple secrets in different
	// target namespaces don't collide as bundle resource names
	if name, _ := r1["name"].(string); name == "" {
		t.Errorf("resources[1].name is empty")
	}
}

func TestBundleClient_ApplySecretIdempotent(t *testing.T) {
	scheme := newBundleTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	bc := cluster.NewBundleClient(c, scheme, cluster.BundleClientOptions{
		ClusterID: "c-abc", FleetWorkspace: "fleet-default", OwnerName: "wl", OwnerNamespace: "default",
	})
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ngc-secret", Namespace: "target-ns"},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte(`{"auths":{}}`)},
	}
	for i := 0; i < 3; i++ {
		if err := bc.ApplySecret(context.Background(), sec); err != nil {
			t.Fatalf("ApplySecret #%d: %v", i, err)
		}
	}

	var bundles unstructured.UnstructuredList
	bundles.SetGroupVersionKind(bundleListGVK)
	if err := c.List(context.Background(), &bundles, client.InNamespace("fleet-default")); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(bundles.Items) != 1 {
		t.Errorf("expected exactly 1 bundle after 3 applies, got %d", len(bundles.Items))
	}
}

func TestBundleClient_TwoDifferentSecrets_BothRetained(t *testing.T) {
	scheme := newBundleTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	bc := cluster.NewBundleClient(c, scheme, cluster.BundleClientOptions{
		ClusterID: "c-abc", FleetWorkspace: "fleet-default", OwnerName: "wl", OwnerNamespace: "default",
	})

	pull := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ngc-secret", Namespace: "target-ns"},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data:       map[string][]byte{corev1.DockerConfigJsonKey: []byte(`{"auths":{}}`)},
	}
	api := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ngc-api", Namespace: "target-ns"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"NGC_API_KEY": []byte("nvapi-test")},
	}

	if err := bc.ApplySecret(context.Background(), pull); err != nil {
		t.Fatalf("apply pull: %v", err)
	}
	if err := bc.ApplySecret(context.Background(), api); err != nil {
		t.Fatalf("apply api: %v", err)
	}

	var bundles unstructured.UnstructuredList
	bundles.SetGroupVersionKind(bundleListGVK)
	if err := c.List(context.Background(), &bundles, client.InNamespace("fleet-default")); err != nil {
		t.Fatalf("list: %v", err)
	}
	// Expect 2 bundles, one per secret.
	if len(bundles.Items) != 2 {
		names := make([]string, 0, len(bundles.Items))
		for _, b := range bundles.Items {
			names = append(names, b.GetName())
		}
		t.Errorf("expected 2 bundles (one per secret), got %d: %v", len(bundles.Items), names)
	}
	// Both bundles must reference the expected secrets — assert names map.
	seen := map[string]bool{}
	for _, b := range bundles.Items {
		seen[b.GetName()] = true
	}
	want := []string{"ai-pullsecrets-wl-c-abc-ngc-secret", "ai-pullsecrets-wl-c-abc-ngc-api"}
	for _, n := range want {
		if !seen[n] {
			t.Errorf("missing bundle %q; have %+v", n, seen)
		}
	}
	// Each bundle's spec.resources should hold exactly two resources: the
	// target Namespace manifest followed by the corresponding Secret.
	for _, b := range bundles.Items {
		resources, _, _ := unstructured.NestedSlice(b.Object, "spec", "resources")
		if len(resources) != 2 {
			t.Errorf("bundle %s: expected 2 resources (namespace + secret), got %d", b.GetName(), len(resources))
		}
	}
}

// newBundleTestScheme builds a runtime.Scheme that knows about corev1 (for
// the Secret) and registers the Fleet Bundle GVK so the fake client accepts
// unstructured objects of that kind.
func newBundleTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("add corev1: %v", err)
	}
	// Register Bundle as unstructured so the fake client can roundtrip it.
	s.AddKnownTypeWithName(bundleGVK, &unstructured.Unstructured{})
	s.AddKnownTypeWithName(bundleListGVK, &unstructured.UnstructuredList{})
	return s
}
