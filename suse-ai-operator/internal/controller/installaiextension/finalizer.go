package controller

import (
	"context"

	"github.com/SUSE/suse-ai-operator/internal/logging"
	"helm.sh/helm/v3/pkg/cli"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiplatformv1beta1 "github.com/SUSE/suse-ai-operator/api/v1beta1"
	helmClient "github.com/SUSE/suse-ai-operator/internal/infra/helm"
)

const finalizerName = "ai-platform.suse.com/finalizer"

func (r *InstallAIExtensionReconciler) ensureFinalizer(
	ctx context.Context,
	ext *aiplatformv1beta1.InstallAIExtension,
) (bool, error) {

	log := logging.FromContext(ctx, "finalizer")

	if ContainsString(ext.Finalizers, finalizerName) {
		return false, nil
	}

	log.Info("Adding finalizer")
	patch := client.MergeFrom(ext.DeepCopy())
	ext.Finalizers = append(ext.Finalizers, finalizerName)

	if err := r.Patch(ctx, ext, patch); err != nil {
		return false, err
	}

	return true, nil
}

func (r *InstallAIExtensionReconciler) handleDeletion(
	ctx context.Context,
	ext *aiplatformv1beta1.InstallAIExtension,
	namespace string,
) error {

	log := logging.FromContext(ctx, "finalizer")

	if !ContainsString(ext.Finalizers, finalizerName) {
		return nil
	}

	log.Info("Handling resource deletion")

	settings := cli.New()
	settings.SetNamespace(namespace)

	helm, err := helmClient.New(settings)
	if err != nil {
		log.Error(err, "Failed to create Helm client")
		return err
	}

	// Delete deployment helm release (helm source only)
	deployRelease := ""
	if ext.Spec.Source.Helm != nil {
		deployRelease = deploymentReleaseName(ext)
	} else if ext.Annotations != nil && ext.Annotations[annotationLastSourceType] == aiplatformv1beta1.SourceTypeHelm {
		deployRelease = ext.Annotations[annotationLastHelmRelease]
	}
	if deployRelease != "" {
		if err := helm.DeleteRelease(ctx, deployRelease); err != nil {
			log.Error(err, "Failed to delete deployment release", "release", deployRelease)
			return err
		}
	}

	lastPolicy := ""
	if ext.Annotations != nil {
		lastPolicy = ext.Annotations[annotationLastVersionPolicy]
	}
	if lastPolicy == aiplatformv1beta1.VersionPolicyUnmanaged {
		uiRelease := ext.Spec.Extension.Name
		if ext.Annotations != nil && ext.Annotations[annotationLastUIPluginRelease] != "" {
			uiRelease = ext.Annotations[annotationLastUIPluginRelease]
		}
		if err := helm.DeleteRelease(ctx, uiRelease); err != nil {
			log.Error(err, "Failed to delete UIPlugin release", "release", uiRelease)
			return err
		}
	}

	// Cleanup Rancher resources (always, for both source types)
	if err := r.rancherMgr.Cleanup(ctx, ext, namespace); err != nil {
		log.Error(err, "Failed to cleanup Rancher resources")
		return err
	}

	return r.removeFinalizer(ctx, ext)
}

func (r *InstallAIExtensionReconciler) removeFinalizer(
	ctx context.Context,
	ext *aiplatformv1beta1.InstallAIExtension,
) error {

	log := logging.FromContext(ctx, "finalizer")

	log.Info("Removing finalizer")
	patch := client.MergeFrom(ext.DeepCopy())
	ext.Finalizers = RemoveString(ext.Finalizers, finalizerName)

	if err := r.Patch(ctx, ext, patch); err != nil {
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
