# NVIDIA Blueprint Component Secret Injection — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route Helm-values secret injection per Blueprint component vendor so NVIDIA components (e.g., `k8s-nim-operator`) deploy end-to-end without manual `kubectl create secret` steps.

**Architecture:** Add a `Vendor` enum field to `BlueprintComponent`. The `aiworkload` controller dispatches per vendor through a small `secretInjector` interface: `suseInjector` wraps existing combined-secret behavior unchanged; `nvidiaInjector` creates `ngc-secret` (dockerconfigjson) and `ngc-api` (Opaque, key `NGC_API_KEY`) in the workload's target namespace and writes `imagePullSecrets: [{name}]` plus `image.pullSecrets: [string]` into the rendered Helm values. Frontend wizard auto-fills `vendor` from `AppCollectionItem.library`.

**Tech Stack:** Go 1.x, controller-runtime, kubebuilder, controller-gen, Ginkgo+Gomega (envtest), Vue 3 + TypeScript, yarn.

---

## File Map

**Operator (Go):**
- Modify: `suse-ai-operator/api/v1alpha1/blueprint_types.go` — add `ComponentVendor` type + `Vendor` field.
- Regenerate: `suse-ai-operator/api/v1alpha1/zz_generated.deepcopy.go`.
- Regenerate: `suse-ai-operator/config/crd/bases/ai-platform.suse.com_blueprints.yaml`.
- Regenerate (auto-copied by `make manifests`): `charts/suse-ai-operator/crds/ai-platform.suse.com_blueprints.yaml`.
- Modify: `suse-ai-operator/internal/controller/aiworkload/blueprint.go` — introduce `secretInjector` interface, refactor existing logic into `suseInjector`, add `nvidiaInjector`, add `injectorFor` dispatch; both `ensureBlueprintHelmOp` and `ensureBlueprintGitFile` call the dispatcher.
- Modify: `suse-ai-operator/internal/controller/aiworkload/blueprint_pullsecret_test.go` — add nvidiaInjector + dispatcher tests.

**Frontend (Vue/TS):**
- Modify: `pkg/suse-ai-lifecycle-manager/types/blueprint-types.ts` — add optional `vendor` field on `BlueprintComponent`.
- Modify: `pkg/suse-ai-lifecycle-manager/pages/components/wizard/BlueprintAppSelectorStep.vue:173-180` — set `vendor: app.library` in `addApp`.

---

## Task 1: Add `ComponentVendor` to API and regenerate CRDs

**Files:**
- Modify: `suse-ai-operator/api/v1alpha1/blueprint_types.go`
- Regenerate: `suse-ai-operator/api/v1alpha1/zz_generated.deepcopy.go`
- Regenerate: `suse-ai-operator/config/crd/bases/ai-platform.suse.com_blueprints.yaml`
- Regenerate: `charts/suse-ai-operator/crds/ai-platform.suse.com_blueprints.yaml`

- [ ] **Step 1: Add `ComponentVendor` type and `Vendor` field to `BlueprintComponent`**

Edit `suse-ai-operator/api/v1alpha1/blueprint_types.go`. Above the `BlueprintComponent` struct, add:

```go
// ComponentVendor selects the secret-injection profile for a Blueprint
// component. "suse" preserves the historical combined-secret + global.imagePullSecrets
// behavior. "nvidia" creates ngc-secret + ngc-api in the target namespace
// and writes both common pull-secret value paths.
// +kubebuilder:validation:Enum=suse;nvidia
type ComponentVendor string

const (
	ComponentVendorSUSE   ComponentVendor = "suse"
	ComponentVendorNvidia ComponentVendor = "nvidia"
)
```

In the `BlueprintComponent` struct, add the field (place it just before `Values`):

```go
	// Vendor selects the secret-injection profile. Defaults to "suse" so
	// existing blueprints behave identically after CRD upgrade.
	// +kubebuilder:default=suse
	// +optional
	Vendor ComponentVendor `json:"vendor,omitempty"`
```

- [ ] **Step 2: Regenerate deepcopy + CRDs**

Run from the operator directory:

```bash
cd suse-ai-operator && make manifests generate
```

Expected output: builds `bin/controller-gen` on first run (~30 s), then rewrites `zz_generated.deepcopy.go`, the CRD YAML under `config/crd/bases/`, and copies it to `../charts/suse-ai-operator/crds/`.

- [ ] **Step 3: Verify the regeneration touched the expected files**

```bash
git -C /home/thbertoldi/suse/suse-ai-lifecycle-manager status --short suse-ai-operator/api/v1alpha1 suse-ai-operator/config/crd/bases charts/suse-ai-operator/crds
```

Expected: `M` markers on `blueprint_types.go`, `zz_generated.deepcopy.go`, and both `ai-platform.suse.com_blueprints.yaml` paths.

```bash
grep -A2 '"vendor":' suse-ai-operator/config/crd/bases/ai-platform.suse.com_blueprints.yaml || \
  grep -A2 'vendor:' suse-ai-operator/config/crd/bases/ai-platform.suse.com_blueprints.yaml | head -10
```

Expected: snippet showing `vendor` with `default: suse` and `enum: [suse, nvidia]`.

- [ ] **Step 4: Smoke-build to catch any Go compile errors**

```bash
cd suse-ai-operator && go build ./...
```

Expected: no output (clean build).

- [ ] **Step 5: Commit**

```bash
git add suse-ai-operator/api/v1alpha1/blueprint_types.go \
        suse-ai-operator/api/v1alpha1/zz_generated.deepcopy.go \
        suse-ai-operator/config/crd/bases/ai-platform.suse.com_blueprints.yaml \
        charts/suse-ai-operator/crds/ai-platform.suse.com_blueprints.yaml
git commit -m "feat: add Vendor field to BlueprintComponent"
```

---

## Task 2: Extract `secretInjector` interface and `suseInjector` (refactor, no behavior change)

**Files:**
- Modify: `suse-ai-operator/internal/controller/aiworkload/blueprint.go`

This task is a pure refactor. The existing `TestEnsureCombinedPullSecret_*` tests are the regression guard — they must continue to pass at every step.

- [ ] **Step 1: Run the existing tests as a baseline**

```bash
cd suse-ai-operator && go test ./internal/controller/aiworkload/ -run 'TestEnsureCombinedPullSecret' -count=1 -v
```

Expected: PASS for `TestEnsureCombinedPullSecret_IncludesNvidia` and `TestEnsureCombinedPullSecret_NvidiaHostOverride`.

- [ ] **Step 2: Add the `secretInjector` interface and `suseInjector` wrapper near the top of `blueprint.go`**

In `suse-ai-operator/internal/controller/aiworkload/blueprint.go`, just below the `const (...defaultAppCollectionHost... combinedPullSecretName...)` block (around line 184–189 today), add:

```go
// secretInjector configures Helm values for a blueprint component so its
// rendered workloads can pull images and access vendor APIs. Each implementation
// owns the namespace-scoped Secret objects it requires and the Helm-values paths
// it writes. A no-op Apply (e.g., missing credentials) is acceptable; Helm will
// surface the resulting ImagePullBackOff downstream.
type secretInjector interface {
	Apply(ctx context.Context, targetNamespace string, repoInfo clusterRepoInfo, vals map[string]any) error
}

// suseInjector preserves the historical combined-secret behavior: one
// dockerconfigjson covering every configured registry, written into both
// imagePullSecrets and global.imagePullSecrets.
type suseInjector struct{ r *AIWorkloadReconciler }

func (s *suseInjector) Apply(ctx context.Context, targetNamespace string, repoInfo clusterRepoInfo, vals map[string]any) error {
	name, err := s.r.ensureCombinedPullSecret(ctx, targetNamespace, repoInfo)
	if err != nil {
		log.FromContext(ctx).Error(err, "could not create image pull secret", "namespace", targetNamespace)
		return nil
	}
	if name == "" {
		return nil
	}
	pullSecrets := []any{map[string]any{"name": name}}
	vals["imagePullSecrets"] = pullSecrets
	vals["global"] = map[string]any{"imagePullSecrets": pullSecrets}
	return nil
}

// injectorFor returns the secretInjector for a component vendor. Unknown or
// empty vendors fall back to the SUSE profile defensively; the CRD default
// fills the field in practice.
func (r *AIWorkloadReconciler) injectorFor(vendor aiplatformv1alpha1.ComponentVendor) secretInjector {
	switch vendor {
	case aiplatformv1alpha1.ComponentVendorNvidia:
		return &nvidiaInjector{r: r}
	default:
		return &suseInjector{r: r}
	}
}
```

Note: `nvidiaInjector` is forward-referenced; it's added in Task 3. The build will fail between Steps 2 and 3 — that's expected; we stage in two commits for clarity once Task 3 is in.

- [ ] **Step 3: Add a temporary stub for `nvidiaInjector` to keep the build green**

Append to `blueprint.go`:

```go
// nvidiaInjector is implemented in the next task; this stub keeps the build
// green during the refactor and is replaced by the real implementation in
// the same PR.
type nvidiaInjector struct{ r *AIWorkloadReconciler }

func (n *nvidiaInjector) Apply(ctx context.Context, targetNamespace string, repoInfo clusterRepoInfo, vals map[string]any) error {
	return nil
}
```

- [ ] **Step 4: Replace the inlined logic in `ensureBlueprintHelmOp` with a dispatcher call**

In `suse-ai-operator/internal/controller/aiworkload/blueprint.go`, find the block (currently lines 118–126):

```go
	pullSecretName, err := r.ensureCombinedPullSecret(ctx, w.Spec.TargetNamespace, repoInfo)
	if err != nil {
		log.FromContext(ctx).Error(err, "could not create image pull secret", "namespace", w.Spec.TargetNamespace)
	}
	if pullSecretName != "" {
		pullSecrets := []any{map[string]any{"name": pullSecretName}}
		vals["imagePullSecrets"] = pullSecrets
		vals["global"] = map[string]any{"imagePullSecrets": pullSecrets}
	}
```

Replace it with:

```go
	if err := r.injectorFor(c.Vendor).Apply(ctx, w.Spec.TargetNamespace, repoInfo, vals); err != nil {
		return fmt.Errorf("inject secrets for %s: %w", c.ChartName, err)
	}
```

- [ ] **Step 5: Replace the same block in `ensureBlueprintGitFile`**

In the same file, find the block currently lines 356–364:

```go
	pullSecretName, err := r.ensureCombinedPullSecret(ctx, w.Spec.TargetNamespace, repoInfo)
	if err != nil {
		log.FromContext(ctx).Error(err, "could not create image pull secret", "namespace", w.Spec.TargetNamespace)
	}
	if pullSecretName != "" {
		pullSecrets := []any{map[string]any{"name": pullSecretName}}
		vals["imagePullSecrets"] = pullSecrets
		vals["global"] = map[string]any{"imagePullSecrets": pullSecrets}
	}
```

Replace it with:

```go
	if err := r.injectorFor(c.Vendor).Apply(ctx, w.Spec.TargetNamespace, repoInfo, vals); err != nil {
		return fmt.Errorf("inject secrets for %s: %w", c.ChartName, err)
	}
```

- [ ] **Step 6: Build and run the existing regression tests**

```bash
cd suse-ai-operator && go build ./... && \
  go test ./internal/controller/aiworkload/ -run 'TestEnsureCombinedPullSecret' -count=1 -v
```

Expected: PASS for both pre-existing tests. The refactor must not change observable behavior. `ensureCombinedPullSecret` is still a public-ish package function exercised directly by tests — that signature is untouched.

- [ ] **Step 7: Commit**

```bash
git add suse-ai-operator/internal/controller/aiworkload/blueprint.go
git commit -m "refactor: extract secretInjector interface from blueprint.go

Wraps existing combined-secret logic in suseInjector; adds nvidiaInjector
stub + injectorFor dispatcher. No behavior change; regression tests pass."
```

---

## Task 3: `nvidiaInjector` — create both secrets

**Files:**
- Modify: `suse-ai-operator/internal/controller/aiworkload/blueprint.go`
- Modify: `suse-ai-operator/internal/controller/aiworkload/blueprint_pullsecret_test.go`

- [ ] **Step 1: Add NVIDIA constants near the existing const block**

In `blueprint.go`, extend the existing const block:

```go
const (
	defaultAppCollectionHost = "dp.apps.rancher.io"
	defaultSUSERegistryHost  = "registry.suse.com"
	defaultNvidiaHost        = "nvcr.io"
	combinedPullSecretName   = "suse-ai-pull-combined"

	nvidiaImagePullSecretName = "ngc-secret"
	nvidiaAPISecretName       = "ngc-api"
	nvidiaAPISecretKey        = "NGC_API_KEY"
)
```

- [ ] **Step 2: Write the failing secret-creation test**

Append to `suse-ai-operator/internal/controller/aiworkload/blueprint_pullsecret_test.go`:

```go
func TestNvidiaInjector_CreatesBothSecrets(t *testing.T) {
	const opNS = "suse-ai-operator"
	const targetNS = "rag"

	scheme := kruntime.NewScheme()
	if err := aiplatformv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add aiplatform scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}

	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ngc-user", Namespace: opNS},
		Data:       map[string][]byte{"username": []byte("$oauthtoken")},
	}
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ngc-token", Namespace: opNS},
		Data:       map[string][]byte{"token": []byte("nvapi-xyz")},
	}
	settings := &aiplatformv1alpha1.Settings{
		ObjectMeta: metav1.ObjectMeta{Name: operatorSettingsName, Namespace: opNS},
		Spec: aiplatformv1alpha1.SettingsSpec{
			Nvidia: aiplatformv1alpha1.NvidiaSettings{
				UserSecretRef:  &aiplatformv1alpha1.SecretKeyRef{Name: "ngc-user", Key: "username"},
				TokenSecretRef: &aiplatformv1alpha1.SecretKeyRef{Name: "ngc-token", Key: "token"},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(userSecret, tokenSecret, settings).Build()
	r := &AIWorkloadReconciler{Client: c, OperatorNamespace: opNS}
	inj := &nvidiaInjector{r: r}

	vals := map[string]any{}
	if err := inj.Apply(context.Background(), targetNS, clusterRepoInfo{}, vals); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	pull := &corev1.Secret{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: targetNS, Name: nvidiaImagePullSecretName}, pull); err != nil {
		t.Fatalf("get %s: %v", nvidiaImagePullSecretName, err)
	}
	if pull.Type != corev1.SecretTypeDockerConfigJson {
		t.Errorf("ngc-secret type = %v, want %v", pull.Type, corev1.SecretTypeDockerConfigJson)
	}
	var cfg struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(pull.Data[corev1.DockerConfigJsonKey], &cfg); err != nil {
		t.Fatalf("parse dockerconfigjson: %v", err)
	}
	entry, ok := cfg.Auths["nvcr.io"]
	if !ok {
		t.Fatalf("expected nvcr.io entry, got: %v", cfg.Auths)
	}
	decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if string(decoded) != "$oauthtoken:nvapi-xyz" {
		t.Errorf("auth payload = %q, want %q", string(decoded), "$oauthtoken:nvapi-xyz")
	}

	api := &corev1.Secret{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: targetNS, Name: nvidiaAPISecretName}, api); err != nil {
		t.Fatalf("get %s: %v", nvidiaAPISecretName, err)
	}
	if api.Type != corev1.SecretTypeOpaque {
		t.Errorf("ngc-api type = %v, want %v", api.Type, corev1.SecretTypeOpaque)
	}
	if got := string(api.Data[nvidiaAPISecretKey]); got != "nvapi-xyz" {
		t.Errorf("NGC_API_KEY = %q, want %q", got, "nvapi-xyz")
	}
}
```

- [ ] **Step 3: Run the test, expect failure**

```bash
cd suse-ai-operator && go test ./internal/controller/aiworkload/ -run 'TestNvidiaInjector_CreatesBothSecrets' -count=1 -v
```

Expected: FAIL — the stub `Apply` returns nil without creating anything, so `c.Get` for `ngc-secret` returns NotFound.

- [ ] **Step 4: Replace the `nvidiaInjector` stub with a real implementation**

In `blueprint.go`, replace the stub `Apply` from Task 2 Step 3 with:

```go
// nvidiaInjector creates the conventional ngc-secret + ngc-api in the target
// namespace and writes both common pull-secret value paths. NVIDIA charts honor
// either the standard k8s pod-spec list-of-objects shape (imagePullSecrets) or
// the k8s-nim-operator flat-string shape (image.pullSecrets); writing both
// covers the surveyed NIM chart families.
type nvidiaInjector struct{ r *AIWorkloadReconciler }

func (n *nvidiaInjector) Apply(ctx context.Context, targetNamespace string, repoInfo clusterRepoInfo, vals map[string]any) error {
	l := log.FromContext(ctx)

	var s aiplatformv1alpha1.Settings
	if err := n.r.Get(ctx, types.NamespacedName{Namespace: n.r.OperatorNamespace, Name: operatorSettingsName}, &s); err != nil {
		l.Info("nvidia injector: settings not found, skipping", "namespace", targetNamespace, "err", err.Error())
		return nil
	}
	if s.Spec.Nvidia.UserSecretRef == nil || s.Spec.Nvidia.TokenSecretRef == nil {
		l.Info("nvidia injector: credentials not configured, skipping", "namespace", targetNamespace)
		return nil
	}
	user, err := n.r.readSettingsSecretKey(ctx, s.Spec.Nvidia.UserSecretRef)
	if err != nil || user == "" {
		l.Info("nvidia injector: user secret unreadable, skipping", "namespace", targetNamespace, "err", fmt.Sprint(err))
		return nil
	}
	token, err := n.r.readSettingsSecretKey(ctx, s.Spec.Nvidia.TokenSecretRef)
	if err != nil || token == "" {
		l.Info("nvidia injector: token secret unreadable, skipping", "namespace", targetNamespace, "err", fmt.Sprint(err))
		return nil
	}

	host := defaultNvidiaHost
	if s.Spec.RegistryEndpoints != nil && s.Spec.RegistryEndpoints.Nvidia != "" {
		host = s.Spec.RegistryEndpoints.Nvidia
	}

	dockerCfg, err := json.Marshal(map[string]any{
		"auths": map[string]any{host: dockerAuthEntry(user, token)},
	})
	if err != nil {
		return fmt.Errorf("marshal ngc dockerconfigjson: %w", err)
	}

	pullSecret := &corev1.Secret{}
	pullSecret.APIVersion = "v1"
	pullSecret.Kind = "Secret"
	pullSecret.Name = nvidiaImagePullSecretName
	pullSecret.Namespace = targetNamespace
	pullSecret.Type = corev1.SecretTypeDockerConfigJson
	pullSecret.Data = map[string][]byte{corev1.DockerConfigJsonKey: dockerCfg}
	if err := n.r.Patch(ctx, pullSecret, client.Apply, client.ForceOwnership, client.FieldOwner("suse-ai-operator")); err != nil {
		return fmt.Errorf("patch %s/%s: %w", targetNamespace, nvidiaImagePullSecretName, err)
	}

	apiSecret := &corev1.Secret{}
	apiSecret.APIVersion = "v1"
	apiSecret.Kind = "Secret"
	apiSecret.Name = nvidiaAPISecretName
	apiSecret.Namespace = targetNamespace
	apiSecret.Type = corev1.SecretTypeOpaque
	apiSecret.Data = map[string][]byte{nvidiaAPISecretKey: []byte(token)}
	if err := n.r.Patch(ctx, apiSecret, client.Apply, client.ForceOwnership, client.FieldOwner("suse-ai-operator")); err != nil {
		return fmt.Errorf("patch %s/%s: %w", targetNamespace, nvidiaAPISecretName, err)
	}

	// Values injection added in Task 6.
	_ = vals
	return nil
}
```

- [ ] **Step 5: Run the test, expect pass**

```bash
cd suse-ai-operator && go test ./internal/controller/aiworkload/ -run 'TestNvidiaInjector_CreatesBothSecrets' -count=1 -v
```

Expected: PASS.

- [ ] **Step 6: Run the full file's tests to confirm no regression**

```bash
cd suse-ai-operator && go test ./internal/controller/aiworkload/ -run 'TestEnsureCombinedPullSecret|TestNvidiaInjector' -count=1 -v
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add suse-ai-operator/internal/controller/aiworkload/blueprint.go \
        suse-ai-operator/internal/controller/aiworkload/blueprint_pullsecret_test.go
git commit -m "feat: nvidiaInjector creates ngc-secret and ngc-api in target namespace"
```

---

## Task 4: `nvidiaInjector` — registry host override

**Files:**
- Modify: `suse-ai-operator/internal/controller/aiworkload/blueprint_pullsecret_test.go`

- [ ] **Step 1: Add the host-override test**

Append to `blueprint_pullsecret_test.go`:

```go
func TestNvidiaInjector_HostOverride(t *testing.T) {
	const opNS = "suse-ai-operator"
	const targetNS = "rag"
	const customHost = "mirror.example.com"

	scheme := kruntime.NewScheme()
	if err := aiplatformv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add aiplatform scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}

	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ngc-user", Namespace: opNS},
		Data:       map[string][]byte{"username": []byte("$oauthtoken")},
	}
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ngc-token", Namespace: opNS},
		Data:       map[string][]byte{"token": []byte("nvapi-xyz")},
	}
	settings := &aiplatformv1alpha1.Settings{
		ObjectMeta: metav1.ObjectMeta{Name: operatorSettingsName, Namespace: opNS},
		Spec: aiplatformv1alpha1.SettingsSpec{
			RegistryEndpoints: &aiplatformv1alpha1.RegistryEndpointsSettings{Nvidia: customHost},
			Nvidia: aiplatformv1alpha1.NvidiaSettings{
				UserSecretRef:  &aiplatformv1alpha1.SecretKeyRef{Name: "ngc-user", Key: "username"},
				TokenSecretRef: &aiplatformv1alpha1.SecretKeyRef{Name: "ngc-token", Key: "token"},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(userSecret, tokenSecret, settings).Build()
	r := &AIWorkloadReconciler{Client: c, OperatorNamespace: opNS}
	inj := &nvidiaInjector{r: r}

	if err := inj.Apply(context.Background(), targetNS, clusterRepoInfo{}, map[string]any{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	pull := &corev1.Secret{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: targetNS, Name: nvidiaImagePullSecretName}, pull); err != nil {
		t.Fatalf("get pull secret: %v", err)
	}
	var cfg struct {
		Auths map[string]struct{ Auth string `json:"auth"` } `json:"auths"`
	}
	if err := json.Unmarshal(pull.Data[corev1.DockerConfigJsonKey], &cfg); err != nil {
		t.Fatalf("parse dockerconfigjson: %v", err)
	}
	if _, ok := cfg.Auths[customHost]; !ok {
		t.Errorf("expected %q auth entry, got %v", customHost, cfg.Auths)
	}
	if _, ok := cfg.Auths["nvcr.io"]; ok {
		t.Errorf("did not expect default nvcr.io entry when override set, got %v", cfg.Auths)
	}
}
```

- [ ] **Step 2: Run it, expect pass (host override already implemented in Task 3)**

```bash
cd suse-ai-operator && go test ./internal/controller/aiworkload/ -run 'TestNvidiaInjector_HostOverride' -count=1 -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add suse-ai-operator/internal/controller/aiworkload/blueprint_pullsecret_test.go
git commit -m "test: nvidiaInjector honors RegistryEndpoints.Nvidia host override"
```

---

## Task 5: `nvidiaInjector` — no-credentials no-op behavior

**Files:**
- Modify: `suse-ai-operator/internal/controller/aiworkload/blueprint_pullsecret_test.go`

- [ ] **Step 1: Add two no-op tests**

Append to `blueprint_pullsecret_test.go`:

```go
func TestNvidiaInjector_NoCreds_NoOp(t *testing.T) {
	const opNS = "suse-ai-operator"
	const targetNS = "rag"

	scheme := kruntime.NewScheme()
	if err := aiplatformv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add aiplatform scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	settings := &aiplatformv1alpha1.Settings{
		ObjectMeta: metav1.ObjectMeta{Name: operatorSettingsName, Namespace: opNS},
		Spec:       aiplatformv1alpha1.SettingsSpec{}, // no Nvidia creds
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(settings).Build()
	r := &AIWorkloadReconciler{Client: c, OperatorNamespace: opNS}
	inj := &nvidiaInjector{r: r}

	vals := map[string]any{}
	if err := inj.Apply(context.Background(), targetNS, clusterRepoInfo{}, vals); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(vals) != 0 {
		t.Errorf("vals was mutated despite missing creds: %v", vals)
	}
	pull := &corev1.Secret{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: targetNS, Name: nvidiaImagePullSecretName}, pull); err == nil {
		t.Errorf("ngc-secret should not exist when creds are missing")
	}
}

func TestNvidiaInjector_MissingTokenSecret(t *testing.T) {
	const opNS = "suse-ai-operator"
	const targetNS = "rag"

	scheme := kruntime.NewScheme()
	if err := aiplatformv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add aiplatform scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	settings := &aiplatformv1alpha1.Settings{
		ObjectMeta: metav1.ObjectMeta{Name: operatorSettingsName, Namespace: opNS},
		Spec: aiplatformv1alpha1.SettingsSpec{
			Nvidia: aiplatformv1alpha1.NvidiaSettings{
				UserSecretRef:  &aiplatformv1alpha1.SecretKeyRef{Name: "missing", Key: "username"},
				TokenSecretRef: &aiplatformv1alpha1.SecretKeyRef{Name: "missing", Key: "token"},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(settings).Build()
	r := &AIWorkloadReconciler{Client: c, OperatorNamespace: opNS}
	inj := &nvidiaInjector{r: r}

	if err := inj.Apply(context.Background(), targetNS, clusterRepoInfo{}, map[string]any{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	pull := &corev1.Secret{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: targetNS, Name: nvidiaImagePullSecretName}, pull); err == nil {
		t.Errorf("ngc-secret should not exist when referenced secret is missing")
	}
}
```

- [ ] **Step 2: Run, expect pass (no-op behavior already implemented)**

```bash
cd suse-ai-operator && go test ./internal/controller/aiworkload/ -run 'TestNvidiaInjector_NoCreds_NoOp|TestNvidiaInjector_MissingTokenSecret' -count=1 -v
```

Expected: both PASS.

- [ ] **Step 3: Commit**

```bash
git add suse-ai-operator/internal/controller/aiworkload/blueprint_pullsecret_test.go
git commit -m "test: nvidiaInjector is no-op when credentials are missing"
```

---

## Task 6: `nvidiaInjector` — values injection (both pull-secret path shapes)

**Files:**
- Modify: `suse-ai-operator/internal/controller/aiworkload/blueprint.go`
- Modify: `suse-ai-operator/internal/controller/aiworkload/blueprint_pullsecret_test.go`

- [ ] **Step 1: Write the failing values-injection test**

Append to `blueprint_pullsecret_test.go`:

```go
func TestNvidiaInjector_WritesBothPathShapes(t *testing.T) {
	const opNS = "suse-ai-operator"
	const targetNS = "rag"

	scheme := kruntime.NewScheme()
	if err := aiplatformv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add aiplatform scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ngc-user", Namespace: opNS},
		Data:       map[string][]byte{"username": []byte("$oauthtoken")},
	}
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ngc-token", Namespace: opNS},
		Data:       map[string][]byte{"token": []byte("nvapi-xyz")},
	}
	settings := &aiplatformv1alpha1.Settings{
		ObjectMeta: metav1.ObjectMeta{Name: operatorSettingsName, Namespace: opNS},
		Spec: aiplatformv1alpha1.SettingsSpec{
			Nvidia: aiplatformv1alpha1.NvidiaSettings{
				UserSecretRef:  &aiplatformv1alpha1.SecretKeyRef{Name: "ngc-user", Key: "username"},
				TokenSecretRef: &aiplatformv1alpha1.SecretKeyRef{Name: "ngc-token", Key: "token"},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(userSecret, tokenSecret, settings).Build()
	r := &AIWorkloadReconciler{Client: c, OperatorNamespace: opNS}
	inj := &nvidiaInjector{r: r}

	vals := map[string]any{}
	if err := inj.Apply(context.Background(), targetNS, clusterRepoInfo{}, vals); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Standard k8s pod-spec shape at top level.
	topList, ok := vals["imagePullSecrets"].([]any)
	if !ok || len(topList) != 1 {
		t.Fatalf("imagePullSecrets = %#v, want one entry", vals["imagePullSecrets"])
	}
	entry, ok := topList[0].(map[string]any)
	if !ok || entry["name"] != nvidiaImagePullSecretName {
		t.Errorf("imagePullSecrets[0] = %#v, want {name: %q}", topList[0], nvidiaImagePullSecretName)
	}

	// k8s-nim-operator's flat-string shape.
	image, ok := vals["image"].(map[string]any)
	if !ok {
		t.Fatalf("image = %#v, want map", vals["image"])
	}
	imgList, ok := image["pullSecrets"].([]any)
	if !ok || len(imgList) != 1 || imgList[0] != nvidiaImagePullSecretName {
		t.Errorf("image.pullSecrets = %#v, want [%q]", image["pullSecrets"], nvidiaImagePullSecretName)
	}

	// Must not touch global.
	if _, ok := vals["global"]; ok {
		t.Errorf("global key should not be set by nvidiaInjector, got %#v", vals["global"])
	}
}
```

- [ ] **Step 2: Run, expect failure (no values mutation yet)**

```bash
cd suse-ai-operator && go test ./internal/controller/aiworkload/ -run 'TestNvidiaInjector_WritesBothPathShapes' -count=1 -v
```

Expected: FAIL — `vals["imagePullSecrets"]` is nil.

- [ ] **Step 3: Replace the `_ = vals` placeholder in `nvidiaInjector.Apply` with a call to the new helper**

In `blueprint.go`, find the single line `_ = vals` near the bottom of `nvidiaInjector.Apply` (added in Task 3 Step 4) and replace exactly that one line with:

```go
	injectNvidiaPullSecretRefs(vals)
```

The `// Values injection added in Task 6.` comment above it can stay or be deleted; either is fine. The `return nil` and closing `}` that follow on the next lines are unchanged.

- [ ] **Step 4: Append the helper functions to `blueprint.go`**

Append the following at the bottom of `blueprint.go`:

```go
// injectNvidiaPullSecretRefs writes the ngc-secret reference into both common
// pull-secret value paths used by NVIDIA charts. Merge rules:
//   - path absent → create with [ngc-secret]
//   - path present and ngc-secret already listed → leave unchanged
//   - path present with other entries → prepend ngc-secret
//   - path present with an unexpected shape → leave untouched (author intent)
func injectNvidiaPullSecretRefs(vals map[string]any) {
	// Top-level k8s pod-spec shape: list of objects with "name".
	switch existing := vals["imagePullSecrets"].(type) {
	case nil:
		vals["imagePullSecrets"] = []any{map[string]any{"name": nvidiaImagePullSecretName}}
	case []any:
		if !containsObjectNamed(existing, nvidiaImagePullSecretName) {
			vals["imagePullSecrets"] = append([]any{map[string]any{"name": nvidiaImagePullSecretName}}, existing...)
		}
	}

	// k8s-nim-operator shape: image.pullSecrets is a flat string list. Only
	// create the parent map if values["image"] is absent or already a map; if
	// it's something unexpected, leave it alone.
	imageRaw, present := vals["image"]
	if !present {
		vals["image"] = map[string]any{"pullSecrets": []any{nvidiaImagePullSecretName}}
		return
	}
	image, ok := imageRaw.(map[string]any)
	if !ok {
		return
	}
	switch existing := image["pullSecrets"].(type) {
	case nil:
		image["pullSecrets"] = []any{nvidiaImagePullSecretName}
	case []any:
		if !containsString(existing, nvidiaImagePullSecretName) {
			image["pullSecrets"] = append([]any{nvidiaImagePullSecretName}, existing...)
		}
	}
}

func containsObjectNamed(list []any, name string) bool {
	for _, item := range list {
		if obj, ok := item.(map[string]any); ok && obj["name"] == name {
			return true
		}
	}
	return false
}

func containsString(list []any, s string) bool {
	for _, item := range list {
		if v, ok := item.(string); ok && v == s {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Run the test, expect pass**

```bash
cd suse-ai-operator && go test ./internal/controller/aiworkload/ -run 'TestNvidiaInjector_WritesBothPathShapes' -count=1 -v
```

Expected: PASS.

- [ ] **Step 6: Run the suite so far**

```bash
cd suse-ai-operator && go test ./internal/controller/aiworkload/ -run 'TestEnsureCombinedPullSecret|TestNvidiaInjector' -count=1 -v
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add suse-ai-operator/internal/controller/aiworkload/blueprint.go \
        suse-ai-operator/internal/controller/aiworkload/blueprint_pullsecret_test.go
git commit -m "feat: nvidiaInjector writes both pull-secret value path shapes"
```

---

## Task 7: `nvidiaInjector` — merge semantics (preserve author values, idempotent)

**Files:**
- Modify: `suse-ai-operator/internal/controller/aiworkload/blueprint_pullsecret_test.go`

- [ ] **Step 1: Add three merge-semantics tests**

Append to `blueprint_pullsecret_test.go`:

```go
func TestNvidiaInjector_PreservesAuthorPullSecrets(t *testing.T) {
	c, r := buildNvidiaInjectorFixture(t)
	inj := &nvidiaInjector{r: r}

	vals := map[string]any{
		"imagePullSecrets": []any{map[string]any{"name": "author-secret"}},
		"image":            map[string]any{"pullSecrets": []any{"author-string"}},
	}
	if err := inj.Apply(context.Background(), "rag", clusterRepoInfo{}, vals); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	topList := vals["imagePullSecrets"].([]any)
	if len(topList) != 2 ||
		topList[0].(map[string]any)["name"] != nvidiaImagePullSecretName ||
		topList[1].(map[string]any)["name"] != "author-secret" {
		t.Errorf("imagePullSecrets = %#v, want [ngc-secret, author-secret]", topList)
	}
	imgList := vals["image"].(map[string]any)["pullSecrets"].([]any)
	if len(imgList) != 2 || imgList[0] != nvidiaImagePullSecretName || imgList[1] != "author-string" {
		t.Errorf("image.pullSecrets = %#v, want [ngc-secret, author-string]", imgList)
	}
	_ = c
}

func TestNvidiaInjector_IdempotentSelfEntry(t *testing.T) {
	c, r := buildNvidiaInjectorFixture(t)
	inj := &nvidiaInjector{r: r}

	vals := map[string]any{}
	if err := inj.Apply(context.Background(), "rag", clusterRepoInfo{}, vals); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if err := inj.Apply(context.Background(), "rag", clusterRepoInfo{}, vals); err != nil {
		t.Fatalf("second Apply: %v", err)
	}

	topList := vals["imagePullSecrets"].([]any)
	if len(topList) != 1 {
		t.Errorf("imagePullSecrets duplicated after re-Apply: %#v", topList)
	}
	imgList := vals["image"].(map[string]any)["pullSecrets"].([]any)
	if len(imgList) != 1 {
		t.Errorf("image.pullSecrets duplicated after re-Apply: %#v", imgList)
	}
	_ = c
}

func TestNvidiaInjector_LeavesUnexpectedShapesAlone(t *testing.T) {
	c, r := buildNvidiaInjectorFixture(t)
	inj := &nvidiaInjector{r: r}

	// Author wrote an integer where we expect a slice — refuse to mutate.
	vals := map[string]any{"imagePullSecrets": 42}
	if err := inj.Apply(context.Background(), "rag", clusterRepoInfo{}, vals); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if vals["imagePullSecrets"] != 42 {
		t.Errorf("imagePullSecrets was mutated despite unexpected shape: %#v", vals["imagePullSecrets"])
	}
	_ = c
}

// buildNvidiaInjectorFixture sets up a fake client with valid Nvidia
// credentials wired up. Used by tests that focus on values-merge behavior.
func buildNvidiaInjectorFixture(t *testing.T) (client.Client, *AIWorkloadReconciler) {
	t.Helper()
	const opNS = "suse-ai-operator"
	scheme := kruntime.NewScheme()
	if err := aiplatformv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add aiplatform scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ngc-user", Namespace: opNS},
		Data:       map[string][]byte{"username": []byte("$oauthtoken")},
	}
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ngc-token", Namespace: opNS},
		Data:       map[string][]byte{"token": []byte("nvapi-xyz")},
	}
	settings := &aiplatformv1alpha1.Settings{
		ObjectMeta: metav1.ObjectMeta{Name: operatorSettingsName, Namespace: opNS},
		Spec: aiplatformv1alpha1.SettingsSpec{
			Nvidia: aiplatformv1alpha1.NvidiaSettings{
				UserSecretRef:  &aiplatformv1alpha1.SecretKeyRef{Name: "ngc-user", Key: "username"},
				TokenSecretRef: &aiplatformv1alpha1.SecretKeyRef{Name: "ngc-token", Key: "token"},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(userSecret, tokenSecret, settings).Build()
	r := &AIWorkloadReconciler{Client: c, OperatorNamespace: opNS}
	return c, r
}
```

Note: the helper needs `sigs.k8s.io/controller-runtime/pkg/client` imported in the test file. If it's not already present, add `"sigs.k8s.io/controller-runtime/pkg/client"` to the import block.

- [ ] **Step 2: Run the new tests**

```bash
cd suse-ai-operator && go test ./internal/controller/aiworkload/ -run 'TestNvidiaInjector_PreservesAuthorPullSecrets|TestNvidiaInjector_IdempotentSelfEntry|TestNvidiaInjector_LeavesUnexpectedShapesAlone' -count=1 -v
```

Expected: all PASS (the implementation in Task 6 Step 3 already handles these cases).

- [ ] **Step 3: Commit**

```bash
git add suse-ai-operator/internal/controller/aiworkload/blueprint_pullsecret_test.go
git commit -m "test: nvidiaInjector preserves author values and is idempotent"
```

---

## Task 8: Dispatcher wiring tests

**Files:**
- Modify: `suse-ai-operator/internal/controller/aiworkload/blueprint_pullsecret_test.go`

- [ ] **Step 1: Add dispatcher unit tests**

Append to `blueprint_pullsecret_test.go`:

```go
func TestInjectorFor_VendorNvidia(t *testing.T) {
	r := &AIWorkloadReconciler{}
	if _, ok := r.injectorFor(aiplatformv1alpha1.ComponentVendorNvidia).(*nvidiaInjector); !ok {
		t.Errorf("vendor nvidia did not yield *nvidiaInjector")
	}
}

func TestInjectorFor_VendorSUSE(t *testing.T) {
	r := &AIWorkloadReconciler{}
	if _, ok := r.injectorFor(aiplatformv1alpha1.ComponentVendorSUSE).(*suseInjector); !ok {
		t.Errorf("vendor suse did not yield *suseInjector")
	}
}

func TestInjectorFor_VendorEmptyDefaultsToSUSE(t *testing.T) {
	r := &AIWorkloadReconciler{}
	if _, ok := r.injectorFor("").(*suseInjector); !ok {
		t.Errorf("empty vendor did not default to *suseInjector")
	}
}
```

- [ ] **Step 2: Run them**

```bash
cd suse-ai-operator && go test ./internal/controller/aiworkload/ -run 'TestInjectorFor_' -count=1 -v
```

Expected: all PASS.

- [ ] **Step 3: Run the entire `aiworkload` package test suite as a final regression sweep**

```bash
cd suse-ai-operator && go test ./internal/controller/aiworkload/... -count=1
```

Expected: all PASS. Note: this triggers the Ginkgo envtest suite (`suite_test.go`), which requires `envtest` binaries; if missing, run `make setup-envtest` first.

- [ ] **Step 4: Commit**

```bash
git add suse-ai-operator/internal/controller/aiworkload/blueprint_pullsecret_test.go
git commit -m "test: injectorFor dispatches per ComponentVendor"
```

---

## Task 9: Frontend — surface `vendor` on `BlueprintComponent`

**Files:**
- Modify: `pkg/suse-ai-lifecycle-manager/types/blueprint-types.ts`
- Modify: `pkg/suse-ai-lifecycle-manager/pages/components/wizard/BlueprintAppSelectorStep.vue`

- [ ] **Step 1: Add `vendor` to the TypeScript type**

Edit `pkg/suse-ai-lifecycle-manager/types/blueprint-types.ts`. Replace the `BlueprintComponent` interface with:

```ts
export type BlueprintComponentVendor = 'suse' | 'nvidia';

export interface BlueprintComponent {
  chartRepo:    string;
  chartName:    string;
  chartVersion: string;
  vendor?:      BlueprintComponentVendor;
  values?:      Record<string, any>;
}
```

- [ ] **Step 2: Auto-fill `vendor` in `addApp`**

Edit `pkg/suse-ai-lifecycle-manager/pages/components/wizard/BlueprintAppSelectorStep.vue`. Find the `emit('update:components', ...)` block in `addApp` (currently lines 173–180) and replace it with:

```ts
  emit('update:components', [
    ...props.components,
    {
      chartRepo,
      chartName:    app.slug_name,
      chartVersion: versions[0] || '1.0.0',
      vendor:       app.library === 'nvidia' ? 'nvidia' : 'suse',
    },
  ]);
```

- [ ] **Step 3: Lint the package**

```bash
cd pkg/suse-ai-lifecycle-manager && yarn lint 2>&1 | tail -20
```

Expected: zero errors. Warnings unrelated to these files are fine.

- [ ] **Step 4: Commit**

```bash
git add pkg/suse-ai-lifecycle-manager/types/blueprint-types.ts \
        pkg/suse-ai-lifecycle-manager/pages/components/wizard/BlueprintAppSelectorStep.vue
git commit -m "feat: persist component vendor on BlueprintComponent

Wizard auto-fills vendor from AppCollectionItem.library so the operator
can route secret injection deterministically per component."
```

---

## Task 10: Final regression sweep and PR-ready state

**Files:** none modified; this task is verification only.

- [ ] **Step 1: Run all operator tests**

```bash
cd suse-ai-operator && go test ./... -count=1
```

Expected: all PASS.

- [ ] **Step 2: Run operator lint**

```bash
cd suse-ai-operator && make lint 2>&1 | tail -20
```

Expected: no findings; if `golangci-lint` is missing, the make target installs it first.

- [ ] **Step 3: Verify the chart CRD matches the source-of-truth CRD**

```bash
diff /home/thbertoldi/suse/suse-ai-lifecycle-manager/suse-ai-operator/config/crd/bases/ai-platform.suse.com_blueprints.yaml \
     /home/thbertoldi/suse/suse-ai-lifecycle-manager/charts/suse-ai-operator/crds/ai-platform.suse.com_blueprints.yaml
```

Expected: no output (files identical — `make manifests` copies them in lockstep).

- [ ] **Step 4: Check the working tree is clean**

```bash
git -C /home/thbertoldi/suse/suse-ai-lifecycle-manager status
```

Expected: branch ahead of `origin/inject-nvidia-auth` by the number of commits from Tasks 1, 2, 3, 4, 5, 6, 7, 8, 9 (approximately 9). Working tree clean except for any untracked files unrelated to this work.

- [ ] **Step 5: Sanity-check the commit log shape**

```bash
git -C /home/thbertoldi/suse/suse-ai-lifecycle-manager log --oneline origin/main..HEAD
```

Expected: each commit is one of: `feat:`, `refactor:`, `test:`, `docs:`. No fixup/wip/squash markers.
