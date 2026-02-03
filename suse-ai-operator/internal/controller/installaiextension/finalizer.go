package controller

import (
	"context"

	"github.com/SUSE/suse-ai-operator/internal/infra/rancher"
	"github.com/SUSE/suse-ai-operator/internal/logging"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiplatformv1alpha1 "github.com/SUSE/suse-ai-operator/api/v1alpha1"
	helmClient "github.com/SUSE/suse-ai-operator/internal/infra/helm"
)

const finalizerName = "ai-platform.suse.com/finalizer"

func (r *InstallAIExtensionReconciler) ensureFinalizer(
	ctx context.Context,
	ext *aiplatformv1alpha1.InstallAIExtension,
) (bool, error) {

	log := logging.FromContext(ctx, "finalizer")

	if ContainsString(ext.Finalizers, finalizerName) {
		return false, nil
	}

	log.Info("Adding finalizer")
	ext.Finalizers = append(ext.Finalizers, finalizerName)

	if err := r.Update(ctx, ext); err != nil {
		return false, err
	}

	return true, nil
}

func (r *InstallAIExtensionReconciler) handleDeletion(
	ctx context.Context,
	ext *aiplatformv1alpha1.InstallAIExtension,
	helm helmClient.HelmClient,
	rancherMgr *rancher.Manager,
	releaseName string,
) error {

	log := logging.FromContext(ctx, "finalizer")

	if !ContainsString(ext.Finalizers, finalizerName) {
		return nil
	}

	log.Info("Handling resource deletion")

	if err := helm.DeleteRelease(ctx, releaseName); err != nil {
		log.Error(err, "Failed to delete Helm release")
		return err
	}

	if err := rancherMgr.Cleanup(ctx, ext); err != nil {
		log.Error(err, "Failed to cleanup Rancher resources")
		return err
	}

	return r.removeFinalizer(ctx, ext)
}

func (r *InstallAIExtensionReconciler) removeFinalizer(
	ctx context.Context,
	ext *aiplatformv1alpha1.InstallAIExtension,
) error {

	log := logging.FromContext(ctx, "finalizer")

	log.Info("Removing finalizer")
	ext.Finalizers = RemoveString(ext.Finalizers, finalizerName)

	if err := r.Update(ctx, ext); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil
		}
		return err
	}

	return nil
}

func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func RemoveString(slice []string, s string) []string {
	result := []string{}
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}
