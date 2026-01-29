package rancher

import (
	"context"

	"github.com/SUSE/suse-ai-operator/api/v1alpha1"
	logging "github.com/SUSE/suse-ai-operator/internal/logging"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (m *Manager) ensureClusterRepo(
	ctx context.Context,
	ext *v1alpha1.InstallAIExtension,
	svcURL string,
) error {
	log := logging.FromContext(ctx, "rancher.clusterrepo").
		WithValues(
			logging.KeyExtension, ext.Name,
			logging.KeyName, ext.Spec.Helm.Name,
		)

	log.Info("Ensuring ClusterRepo")

	repo := &unstructured.Unstructured{}
	repo.SetAPIVersion("catalog.cattle.io/v1")
	repo.SetKind("ClusterRepo")
	repo.SetName(ext.Spec.Helm.Name)

	_, err := ctrl.CreateOrUpdate(ctx, m.client, repo, func() error {
		logging.Trace(log).Info(
			"Setting ClusterRepo URL",
			"url", svcURL,
		)
		return unstructured.SetNestedField(repo.Object, svcURL, "spec", "url")
	})
	if err != nil {
		return err
	}

	logging.Debug(log).Info("ClusterRepo ensured")
	return nil
}

func (m *Manager) deleteClusterRepo(
	ctx context.Context,
	ext *v1alpha1.InstallAIExtension,
) error {
	log := logging.FromContext(ctx, "rancher.clusterrepo").
		WithValues(
			logging.KeyExtension, ext.Name,
			logging.KeyName, ext.Spec.Helm.Name,
		)

	log.Info("Deleting ClusterRepo")

	repo := &unstructured.Unstructured{}
	repo.SetAPIVersion("catalog.cattle.io/v1")
	repo.SetKind("ClusterRepo")
	repo.SetName(ext.Spec.Helm.Name)

	err := m.client.Delete(ctx, repo)
	if client.IgnoreNotFound(err) == nil {
		logging.Debug(log).Info("ClusterRepo already deleted or not found")
		return nil
	}

	if err != nil {
		log.Error(err, "Failed to delete ClusterRepo")
		return err
	}

	log.Info("ClusterRepo deleted")
	return nil
}
