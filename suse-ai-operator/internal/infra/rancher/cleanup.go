package rancher

import (
	"context"

	"github.com/leooamaral/suseai-operator/api/v1alpha1"
	logging "github.com/leooamaral/suseai-operator/internal/logging"
)

func (m *Manager) Cleanup(
	ctx context.Context,
	ext *v1alpha1.InstallAIExtension,
) error {
	log := logging.FromContext(ctx, "rancher.cleanup").
		WithValues(
			logging.KeyExtension, ext.Name,
		)

	log.Info("Cleaning up Rancher resources")
	if ext == nil {
		return nil
	}

	if err := m.deleteUIPlugin(ctx, ext); err != nil {
		return err
	}
	logging.Debug(log).Info("Deleting UIPlugin")

	if ext.Spec.Helm != nil {
		if err := m.deleteClusterRepo(ctx, ext); err != nil {
			return err
		}
		logging.Debug(log).Info("Deleting ClusterRepo")
	}

	log.Info("Rancher cleanup completed")
	return nil
}
