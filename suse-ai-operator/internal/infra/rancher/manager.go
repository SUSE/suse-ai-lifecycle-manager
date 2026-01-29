package rancher

import (
	"context"

	"github.com/SUSE/suse-ai-operator/api/v1alpha1"
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
	ext *v1alpha1.InstallAIExtension,
	svcURL string,
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

	if err := m.ensureUIPlugin(ctx, ext, svcURL); err != nil {
		return err
	}

	log.Info("Rancher resources ensured")
	return nil
}
