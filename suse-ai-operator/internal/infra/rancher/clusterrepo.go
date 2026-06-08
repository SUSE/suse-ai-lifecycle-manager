package rancher

import (
	"context"
	"fmt"

	"github.com/SUSE/suse-ai-operator/api/v1beta1"
	logging "github.com/SUSE/suse-ai-operator/internal/logging"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// add support to gitBranch and gitRepo
func (m *Manager) ensureClusterRepo(
	ctx context.Context,
	ext *v1beta1.InstallAIExtension,
	svcURL string,
) error {
	name := clusterRepoName(ext)

	log := logging.FromContext(ctx, "rancher.clusterrepo").
		WithValues(
			logging.KeyExtension, ext.Name,
			logging.KeyName, name,
		)

	log.Info("Ensuring ClusterRepo")

	repo := &unstructured.Unstructured{}
	repo.SetAPIVersion("catalog.cattle.io/v1")
	repo.SetKind("ClusterRepo")
	repo.SetName(name)

	_, err := ctrl.CreateOrUpdate(ctx, m.client, repo, func() error {
		switch {
		case ext.Spec.Source.Helm != nil:
			logging.Trace(log).Info("Setting ClusterRepo URL", "url", svcURL)
			// Clear git fields in case of source type change
			unstructured.RemoveNestedField(repo.Object, "spec", "gitRepo")
			unstructured.RemoveNestedField(repo.Object, "spec", "gitBranch")
			return unstructured.SetNestedField(repo.Object, svcURL, "spec", "url")

		case ext.Spec.Source.Git != nil:
			logging.Trace(log).Info("Setting ClusterRepo git source",
				"repo", ext.Spec.Source.Git.Repo,
				"branch", ext.Spec.Source.Git.Branch,
			)
			// Clear url field in case of source type change
			unstructured.RemoveNestedField(repo.Object, "spec", "url")
			if err := unstructured.SetNestedField(repo.Object, ext.Spec.Source.Git.Repo, "spec", "gitRepo"); err != nil {
				return err
			}
			return unstructured.SetNestedField(repo.Object, ext.Spec.Source.Git.Branch, "spec", "gitBranch")

		default:
			return fmt.Errorf("source must specify either helm or git")
		}
	})
	if err != nil {
		return err
	}

	logging.Debug(log).Info("ClusterRepo ensured")
	return nil
}

func (m *Manager) deleteClusterRepo(
	ctx context.Context,
	ext *v1beta1.InstallAIExtension,
) error {
	name := clusterRepoName(ext)

	log := logging.FromContext(ctx, "rancher.clusterrepo").
		WithValues(
			logging.KeyExtension, ext.Name,
			logging.KeyName, name,
		)

	log.Info("Deleting ClusterRepo")

	repo := &unstructured.Unstructured{}
	repo.SetAPIVersion("catalog.cattle.io/v1")
	repo.SetKind("ClusterRepo")
	repo.SetName(name)

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

func (m *Manager) DeleteClusterRepoByName(ctx context.Context, name string) error {
	log := logging.FromContext(ctx, "rancher.clusterrepo").
		WithValues(logging.KeyName, name)

	log.Info("Deleting ClusterRepo by name")

	repo := &unstructured.Unstructured{}
	repo.SetAPIVersion("catalog.cattle.io/v1")
	repo.SetKind("ClusterRepo")
	repo.SetName(name)

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

func clusterRepoName(ext *v1beta1.InstallAIExtension) string {
	if ext.Spec.Source.Helm != nil {
		return ext.Spec.Source.Helm.Name
	}
	return ext.Spec.Extension.Name
}
