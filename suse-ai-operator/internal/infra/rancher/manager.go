package rancher

import (
	"context"

	"github.com/SUSE/suse-ai-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/SUSE/suse-ai-operator/internal/infra/helm"
	logging "github.com/SUSE/suse-ai-operator/internal/logging"
)

var requiredCRDs = []string{
	"uiplugins.catalog.cattle.io",
	"clusterrepos.catalog.cattle.io",
}

type Manager struct {
	client     client.Client
	scheme     *runtime.Scheme
	indexCache *helm.IndexCache
}

func NewManager(c client.Client, s *runtime.Scheme) *Manager {
	return &Manager{client: c, scheme: s, indexCache: helm.NewIndexCache()}
}

func (m *Manager) Ensure(
	ctx context.Context,
	ext *v1beta1.InstallAIExtension,
	svcURL string,
	namespace string,
) error {

	log := logging.FromContext(ctx, "rancher").
		WithValues(
			logging.KeyExtension, ext.Name,
			logging.KeyNamespace, ext.Namespace,
		)

	log.Info("Ensuring Rancher resources")

	if err := m.CheckCRDs(ctx, requiredCRDs); err != nil {
		logging.Debug(log).Info("Rancher CRDs not ready yet")
		return err
	}

	if err := m.ensureClusterRepo(ctx, ext, svcURL); err != nil {
		return err
	}

	log.Info("Rancher resources ensured")
	return nil
}

func (m *Manager) EnsureUIPlugin(
	ctx context.Context,
	ext *v1beta1.InstallAIExtension,
	svcURL string,
	namespace string,
) error {
	return m.ensureUIPlugin(ctx, ext, svcURL, namespace)
}

func (m *Manager) DeleteUIPlugin(
	ctx context.Context,
	ext *v1beta1.InstallAIExtension,
	namespace string,
) error {
	return m.deleteUIPlugin(ctx, ext, namespace)
}

func (m *Manager) ResolveLatestVersion(
	ctx context.Context,
	ext *v1beta1.InstallAIExtension,
	svcURL string,
) (string, error) {
	log := logging.FromContext(ctx, "rancher.resolve").
		WithValues(logging.KeyExtension, ext.Spec.Extension.Name)

	indexURLs, err := indexURLsForSource(ext, svcURL)
	if err != nil {
		return "", err
	}

	index, err := getOrFetchIndex(ctx, m.indexCache, indexURLs)
	if err != nil {
		return "", err
	}

	version, err := helm.FindLatestVersion(index, ext.Spec.Extension.Name)
	if err != nil {
		return "", err
	}

	log.Info("Resolved latest version", "version", version)
	return version, nil
}
