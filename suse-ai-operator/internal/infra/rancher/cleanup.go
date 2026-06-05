package rancher

import (
	"context"

	"github.com/SUSE/suse-ai-operator/api/v1beta1"
	logging "github.com/SUSE/suse-ai-operator/internal/logging"
)

func (m *Manager) Cleanup(
	ctx context.Context,
	ext *v1beta1.InstallAIExtension,
	namespace string,
) error {
	if ext == nil {
		return nil
	}

	log := logging.FromContext(ctx, "rancher.cleanup").
		WithValues(
			logging.KeyExtension, ext.Name,
		)

	log.Info("Cleaning up Rancher resources")

	logging.Debug(log).Info("Deleting UIPlugin")
	if err := m.deleteUIPlugin(ctx, ext, namespace); err != nil {
		return err
	}

	logging.Debug(log).Info("Deleting ClusterRepo")
	if err := m.deleteClusterRepo(ctx, ext); err != nil {
		return err
	}

	log.Info("Rancher cleanup completed")
	return nil
}
