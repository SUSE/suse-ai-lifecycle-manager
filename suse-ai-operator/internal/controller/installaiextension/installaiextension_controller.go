/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	urlpkg "net/url"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/cli"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiplatformv1beta1 "github.com/SUSE/suse-ai-operator/api/v1beta1"
	helmClient "github.com/SUSE/suse-ai-operator/internal/infra/helm"
	"github.com/SUSE/suse-ai-operator/internal/infra/kubernetes"
	"github.com/SUSE/suse-ai-operator/internal/infra/rancher"
	"github.com/SUSE/suse-ai-operator/internal/installaiextension"
)

const (
	defaultReadinessTimeout = 5 * time.Minute

	annotationLastSourceType      = "ai-platform.suse.com/last-source-type"
	annotationLastHelmRelease     = "ai-platform.suse.com/last-helm-release-name"
	annotationLastClusterRepo     = "ai-platform.suse.com/last-cluster-repo-name"
	annotationLastUIPluginRelease = "ai-platform.suse.com/last-uiplugin-release-name"
	annotationLastVersionPolicy   = "ai-platform.suse.com/last-version-policy"
	annotationWaitingSince        = "ai-platform.suse.com/waiting-since"
)

// InstallAIExtensionReconciler reconciles a InstallAIExtension object
type InstallAIExtensionReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	Recorder           record.EventRecorder
	Config             *rest.Config
	ExtensionNamespace string
	ReadinessTimeout   time.Duration
	rancherMgr         *rancher.Manager
}

// +kubebuilder:rbac:groups=ai-platform.suse.com,resources=installaiextensions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ai-platform.suse.com,resources=installaiextensions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ai-platform.suse.com,resources=installaiextensions/finalizers,verbs=update
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list

// +kubebuilder:rbac:groups=catalog.cattle.io,resources=clusterrepos,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=catalog.cattle.io,resources=clusterrepos/status,verbs=get;update;patch

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch

func (r *InstallAIExtensionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	namespace := r.ExtensionNamespace

	var installExt aiplatformv1beta1.InstallAIExtension
	if err := r.Get(ctx, req.NamespacedName, &installExt); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion first (for both source types)
	if !installExt.ObjectMeta.DeletionTimestamp.IsZero() {
		if err := r.handleDeletion(ctx, &installExt, namespace); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	added, err := r.ensureFinalizer(ctx, &installExt)
	if err != nil {
		return ctrl.Result{}, err
	}
	if added {
		return ctrl.Result{Requeue: true}, nil
	}

	// Detect source type change and clean up old resources
	if err := r.detectAndCleanupSourceChange(ctx, log, &installExt, namespace); err != nil {
		if setErr := r.setStatus(ctx, &installExt, aiplatformv1beta1.PhaseFailed, err.Error(), ""); setErr != nil {
			log.Error(setErr, "failed to update status")
		}
		return ctrl.Result{}, err
	}

	var svcURL string

	switch {
	case installExt.Spec.Source.Helm != nil:
		svcURL, err = r.reconcileHelmSource(ctx, log, &installExt, namespace)
		if err != nil {
			if setErr := r.setStatus(ctx, &installExt, aiplatformv1beta1.PhaseFailed, err.Error(), ""); setErr != nil {
				log.Error(setErr, "failed to update status")
			}
			return ctrl.Result{}, err
		}

		// Check if deployment is actually ready
		releaseName := deploymentReleaseName(&installExt)
		ready, readyErr := kubernetes.IsDeploymentReady(ctx, r.Client, namespace, releaseName, log)
		if readyErr != nil {
			log.Error(readyErr, "Failed to check deployment readiness", "release", releaseName, "namespace", namespace)
			return r.handleNotReady(ctx, &installExt, log)
		}
		if !ready {
			log.Info("Deployment is not ready yet", "release", releaseName, "namespace", namespace)
			return r.handleNotReady(ctx, &installExt, log)
		}

		if installExt.Annotations != nil && installExt.Annotations[annotationWaitingSince] != "" {
			patch := client.MergeFrom(installExt.DeepCopy())
			delete(installExt.Annotations, annotationWaitingSince)
			if err := r.Patch(ctx, &installExt, patch); err != nil {
				return ctrl.Result{}, err
			}
		}

	case installExt.Spec.Source.Git != nil:
		// Git: no deployment to check

	default:
		return ctrl.Result{}, fmt.Errorf("source must specify either helm or git")
	}

	resolvedExt := &installExt // default: use original
	resolvedVersion := installExt.Spec.Extension.Version
	requeueAfter := time.Duration(0)

	switch installExt.Spec.Extension.VersionPolicy {
	case aiplatformv1beta1.VersionPolicyUnmanaged:
		// No version resolution; ensureUIPluginRelease handles install-once

	default: // managed or empty
		if installExt.Spec.Extension.Version == "" {
			// Git source with no version: resolve latest
			ver, err := r.rancherMgr.ResolveLatestVersion(ctx, &installExt, svcURL)
			if err != nil {
				if setErr := r.setStatus(ctx, &installExt, aiplatformv1beta1.PhaseFailed, err.Error(), ""); setErr != nil {
					log.Error(setErr, "failed to update status")
				}
				return ctrl.Result{}, err
			}
			resolvedVersion = ver
			extCopy := installExt.DeepCopy()
			extCopy.Spec.Extension.Version = resolvedVersion
			resolvedExt = extCopy
			requeueAfter = 5 * time.Minute
		}
	}

	// Ensure ClusterRepo
	if err := r.rancherMgr.Ensure(ctx, resolvedExt, svcURL, namespace); err != nil {
		if setErr := r.setStatus(ctx, &installExt, aiplatformv1beta1.PhaseFailed, err.Error(), ""); setErr != nil {
			log.Error(setErr, "failed to update status")
		}
		return ctrl.Result{}, err
	}

	if resolvedExt.Spec.Extension.VersionPolicy == aiplatformv1beta1.VersionPolicyUnmanaged {
		if err := r.ensureUIPluginRelease(ctx, log, resolvedExt, svcURL, namespace); err != nil {
			if setErr := r.setStatus(ctx, &installExt, aiplatformv1beta1.PhaseFailed, err.Error(), ""); setErr != nil {
				log.Error(setErr, "failed to update status")
			}
			return ctrl.Result{}, err
		}
	} else {
		if err := r.rancherMgr.EnsureUIPlugin(ctx, resolvedExt, svcURL, namespace); err != nil {
			if setErr := r.setStatus(ctx, &installExt, aiplatformv1beta1.PhaseFailed, err.Error(), ""); setErr != nil {
				log.Error(setErr, "failed to update status")
			}
			return ctrl.Result{}, err
		}
	}

	// Track current source type in annotations for future change detection
	if err := r.updateSourceAnnotations(ctx, &installExt); err != nil {
		return ctrl.Result{}, err
	}

	// Everything succeeded — only update status if not already installed
	if installExt.Status.Phase != aiplatformv1beta1.PhaseInstalled || installExt.Status.ResolvedVersion != resolvedVersion {
		msg := fmt.Sprintf("Extension %s installed", installExt.Spec.Extension.Name)
		if err := r.setStatus(ctx, &installExt, aiplatformv1beta1.PhaseInstalled, msg, resolvedVersion); err != nil {
			return ctrl.Result{}, err
		}
	}

	if requeueAfter > 0 {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	return ctrl.Result{}, nil
}

func deploymentReleaseName(ext *aiplatformv1beta1.InstallAIExtension) string {
	return ext.Spec.Source.Helm.Name + "-server"
}

func (r *InstallAIExtensionReconciler) setStatus(
	ctx context.Context,
	ext *aiplatformv1beta1.InstallAIExtension,
	phase, message, resolvedVersion string,
) error {
	patch := client.MergeFrom(ext.DeepCopy())
	ext.Status.Phase = phase
	ext.Status.Message = message
	if resolvedVersion != "" {
		ext.Status.ResolvedVersion = resolvedVersion
	}
	return r.Status().Patch(ctx, ext, patch)
}

func (r *InstallAIExtensionReconciler) handleNotReady(
	ctx context.Context,
	ext *aiplatformv1beta1.InstallAIExtension,
	log logr.Logger,
) (ctrl.Result, error) {

	// Only set installing if not already in that state
	if ext.Status.Phase != aiplatformv1beta1.PhaseInstalling {
		if err := r.setStatus(ctx, ext, aiplatformv1beta1.PhaseInstalling, "Waiting for deployment to be ready", ""); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Use annotation to track when we started waiting, since we no longer have conditions
	var waitingSince time.Time

	if ext.Annotations != nil {
		if ts, ok := ext.Annotations[annotationWaitingSince]; ok {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				waitingSince = t
			}
		}
	}

	if waitingSince.IsZero() {
		waitingSince = time.Now()
		patch := client.MergeFrom(ext.DeepCopy())
		if ext.Annotations == nil {
			ext.Annotations = make(map[string]string)
		}
		ext.Annotations[annotationWaitingSince] = waitingSince.Format(time.RFC3339)
		if err := r.Patch(ctx, ext, patch); err != nil {
			return ctrl.Result{}, err
		}
	}

	if time.Since(waitingSince) > r.ReadinessTimeout {
		if ext.Status.Phase != aiplatformv1beta1.PhaseFailed {
			msg := fmt.Sprintf("Deployment not ready after %s", r.ReadinessTimeout)
			if err := r.setStatus(ctx, ext, aiplatformv1beta1.PhaseFailed, msg, ""); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	elapsed := time.Since(waitingSince).Truncate(time.Second)
	log.Info("Waiting for deployment to be ready", "elapsed", elapsed)
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// --- Source change detection and cleanup ---

func currentSourceType(ext *aiplatformv1beta1.InstallAIExtension) string {
	if ext.Spec.Source.Helm != nil {
		return aiplatformv1beta1.SourceTypeHelm
	}
	if ext.Spec.Source.Git != nil {
		return aiplatformv1beta1.SourceTypeGit
	}
	return ""
}

func (r *InstallAIExtensionReconciler) detectAndCleanupSourceChange(
	ctx context.Context,
	log logr.Logger,
	ext *aiplatformv1beta1.InstallAIExtension,
	namespace string,
) error {
	if ext.Annotations == nil {
		return nil // first reconcile, nothing to clean up
	}

	lastSource := ext.Annotations[annotationLastSourceType]
	if lastSource == "" {
		return nil // no previous source tracked
	}

	current := currentSourceType(ext)

	if lastSource != current {
		log.Info("Source type changed, cleaning up old resources")

		// Clean up old helm release if switching away from helm
		if lastSource == aiplatformv1beta1.SourceTypeHelm {
			oldRelease := ext.Annotations[annotationLastHelmRelease]
			if oldRelease != "" {
				log.Info("Deleting old helm release", "release", oldRelease)
				settings := cli.New()
				settings.SetNamespace(namespace)
				helm, err := helmClient.New(settings)
				if err != nil {
					return fmt.Errorf("failed to create helm client for cleanup: %w", err)
				}
				if err := helm.DeleteRelease(ctx, oldRelease); err != nil {
					return fmt.Errorf("failed to delete old helm release %q: %w", oldRelease, err)
				}
			}
		}
	}

	if current == aiplatformv1beta1.SourceTypeHelm {
		oldRelease := ext.Annotations[annotationLastHelmRelease]
		newRelease := deploymentReleaseName(ext)
		if oldRelease != "" && oldRelease != newRelease {
			log.Info("Helm release name changed, deleting old release", "old", oldRelease, "new", newRelease)
			settings := cli.New()
			settings.SetNamespace(namespace)
			helm, err := helmClient.New(settings)
			if err != nil {
				return fmt.Errorf("failed to create helm client for cleanup: %w", err)
			}
			if err := helm.DeleteRelease(ctx, oldRelease); err != nil {
				return fmt.Errorf("failed to delete old helm release %q: %w", oldRelease, err)
			}
		}
	}

	// Clean up old ClusterRepo if the name changed
	oldClusterRepo := ext.Annotations[annotationLastClusterRepo]
	if oldClusterRepo != "" {
		newClusterRepo := ""
		if ext.Spec.Source.Helm != nil {
			newClusterRepo = ext.Spec.Source.Helm.Name
		} else {
			newClusterRepo = ext.Spec.Extension.Name
		}
		if oldClusterRepo != newClusterRepo {
			log.Info("Deleting old ClusterRepo", "oldName", oldClusterRepo, "newName", newClusterRepo)
			if err := r.rancherMgr.DeleteClusterRepoByName(ctx, oldClusterRepo); err != nil {
				return fmt.Errorf("failed to delete old ClusterRepo %q: %w", oldClusterRepo, err)
			}
		}
	}

	// Clean up UIPlugin on version policy change
	lastPolicy := ext.Annotations[annotationLastVersionPolicy]
	if lastPolicy == "" {
		lastPolicy = aiplatformv1beta1.VersionPolicyManaged
	}
	currentPolicy := ext.Spec.Extension.VersionPolicy
	if currentPolicy == "" {
		currentPolicy = aiplatformv1beta1.VersionPolicyManaged
	}

	if lastPolicy != currentPolicy {
		log.Info("Version policy changed, cleaning up UIPlugin",
			"previousPolicy", lastPolicy, "currentPolicy", currentPolicy)

		if lastPolicy == aiplatformv1beta1.VersionPolicyUnmanaged {
			// unmanaged → managed: delete UIPlugin helm release so ctrl.CreateOrUpdate can take over
			oldUIRelease := ext.Annotations[annotationLastUIPluginRelease]
			if oldUIRelease != "" {
				log.Info("Deleting UIPlugin helm release (switching to managed)", "release", oldUIRelease)
				settings := cli.New()
				settings.SetNamespace(namespace)
				helm, err := helmClient.New(settings)
				if err != nil {
					return fmt.Errorf("failed to create helm client for UIPlugin cleanup: %w", err)
				}
				if err := helm.DeleteRelease(ctx, oldUIRelease); err != nil {
					return fmt.Errorf("failed to delete UIPlugin helm release %q: %w", oldUIRelease, err)
				}
			}
		} else {
			// managed → unmanaged: delete UIPlugin k8s object so helm can create a fresh release
			log.Info("Deleting UIPlugin k8s object (switching to unmanaged)")
			if err := r.rancherMgr.DeleteUIPlugin(ctx, ext, namespace); err != nil {
				return fmt.Errorf("failed to delete UIPlugin for policy change: %w", err)
			}
		}
	}

	return nil
}

func (r *InstallAIExtensionReconciler) updateSourceAnnotations(
	ctx context.Context,
	ext *aiplatformv1beta1.InstallAIExtension,
) error {
	if ext.Annotations == nil {
		ext.Annotations = make(map[string]string)
	}

	current := currentSourceType(ext)

	// Build expected annotations
	expected := map[string]string{
		annotationLastSourceType:      current,
		annotationLastUIPluginRelease: ext.Spec.Extension.Name,
		annotationLastVersionPolicy:   ext.Spec.Extension.VersionPolicy,
	}

	switch {
	case ext.Spec.Source.Helm != nil:
		expected[annotationLastHelmRelease] = deploymentReleaseName(ext)
		expected[annotationLastClusterRepo] = ext.Spec.Source.Helm.Name
	case ext.Spec.Source.Git != nil:
		expected[annotationLastClusterRepo] = ext.Spec.Extension.Name
	}

	// Check if any annotation actually changed
	changed := false
	for k, v := range expected {
		if ext.Annotations[k] != v {
			changed = true
			break
		}
	}
	// Check if helm release annotation needs to be removed (git source)
	if ext.Spec.Source.Git != nil && ext.Annotations[annotationLastHelmRelease] != "" {
		changed = true
	}

	if !changed {
		return nil
	}

	patch := client.MergeFrom(ext.DeepCopy())
	for k, v := range expected {
		ext.Annotations[k] = v
	}
	if ext.Spec.Source.Git != nil {
		delete(ext.Annotations, annotationLastHelmRelease)
	}

	return r.Patch(ctx, ext, patch)
}

// --- Helm source reconciliation ---

func (r *InstallAIExtensionReconciler) reconcileHelmSource(
	ctx context.Context,
	log logr.Logger,
	installExt *aiplatformv1beta1.InstallAIExtension,
	namespace string,
) (string, error) {
	releaseName := deploymentReleaseName(installExt)
	chartVersion := installExt.Spec.Source.Helm.Version
	values, err := helmClient.ConvertHelmValues(installExt.Spec.Source.Helm.Values)
	if err != nil {
		log.Error(err, "failed to convert Helm values")
		return "", err
	}
	url := installExt.Spec.Source.Helm.URL
	u, err := urlpkg.Parse(url)
	if err != nil {
		log.Error(err, "invalid helm url", "url", url)
		return "", err
	}

	var chart string
	switch u.Scheme {
	case "oci", "https":
		chart = url
	default:
		return "", fmt.Errorf("unsupported helm url scheme: %s", u.Scheme)
	}

	settings := cli.New()
	settings.SetNamespace(namespace)

	helm, err := helmClient.New(settings)
	if err != nil {
		log.Error(err, "failed to create Helm client")
		return "", err
	}

	err = helm.EnsureRelease(ctx, helmClient.ReleaseSpec{
		Name:      releaseName,
		Namespace: namespace,
		ChartRef:  chart,
		Version:   chartVersion,
		Values:    values,
	})
	if err != nil {
		return "", err
	}

	svc, err := kubernetes.ServiceForHelmRelease(ctx, r.Client, namespace, releaseName)
	if err != nil {
		log.Info("Error to fetch services")
		return "", err
	}

	svcName, svcNamespace, svcPort, err := installaiextension.ServiceEndpoint(svc)
	if err != nil {
		log.Info("Error to fetch svc info")
		return "", err
	}

	return fmt.Sprintf("http://%s.%s:%d", svcName, svcNamespace, svcPort), nil
}

func (r *InstallAIExtensionReconciler) ensureUIPluginRelease(
	ctx context.Context,
	log logr.Logger,
	ext *aiplatformv1beta1.InstallAIExtension,
	svcURL string,
	namespace string,
) error {
	settings := cli.New()
	settings.SetNamespace(namespace)

	helm, err := helmClient.New(settings)
	if err != nil {
		return err
	}

	info, err := helm.GetRelease(ctx, ext.Spec.Extension.Name)
	if err != nil {
		return fmt.Errorf("failed to check UIPlugin release %q: %w", ext.Spec.Extension.Name, err)
	}
	if info != nil {
		log.Info("UIPlugin release exists, skipping (unmanaged policy)")
		return nil
	}

	// Determine the chart repo URL
	var repoURL string
	switch {
	case ext.Spec.Source.Helm != nil:
		repoURL = svcURL
	case ext.Spec.Source.Git != nil:
		repoURL = rancher.GitRawBaseURL(ext.Spec.Source.Git.Repo, ext.Spec.Source.Git.Branch)
	}

	return helm.EnsureRelease(ctx, helmClient.ReleaseSpec{
		Name:      ext.Spec.Extension.Name,
		Namespace: namespace,
		ChartRef:  ext.Spec.Extension.Name,
		RepoURL:   repoURL,
		Version:   ext.Spec.Extension.Version,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *InstallAIExtensionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.ReadinessTimeout == 0 {
		r.ReadinessTimeout = defaultReadinessTimeout
	}
	r.rancherMgr = rancher.NewManager(r.Client, r.Scheme)
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiplatformv1beta1.InstallAIExtension{}).
		Named("InstallAIExtension").
		Complete(r)
}
