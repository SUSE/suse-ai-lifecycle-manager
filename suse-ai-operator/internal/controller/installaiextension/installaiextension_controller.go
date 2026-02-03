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

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/cli"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiplatformv1alpha1 "github.com/SUSE/suse-ai-operator/api/v1alpha1"
	helmClient "github.com/SUSE/suse-ai-operator/internal/infra/helm"
	"github.com/SUSE/suse-ai-operator/internal/infra/kubernetes"
	"github.com/SUSE/suse-ai-operator/internal/infra/rancher"
	"github.com/SUSE/suse-ai-operator/internal/installaiextension"
)

// InstallAIExtensionReconciler reconciles a InstallAIExtension object
type InstallAIExtensionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Recorder record.EventRecorder
	Config   *rest.Config
}

// +kubebuilder:rbac:groups=ai-platform.suse.com,resources=installaiextensions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ai-platform.suse.com,resources=installaiextensions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ai-platform.suse.com,resources=installaiextensions/finalizers,verbs=update
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list

// +kubebuilder:rbac:groups=catalog.cattle.io,resources=clusterrepos,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=catalog.cattle.io,resources=clusterrepos/status,verbs=get;update;patch

// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the InstallAIExtension object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *InstallAIExtensionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("InstallAIExtension", req.NamespacedName)

	namespace := "cattle-ui-plugin-system"

	var svcURL string

	var installExt aiplatformv1alpha1.InstallAIExtension
	if err := r.Get(ctx, req.NamespacedName, &installExt); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// namespace := installExt.Spec.Helm.Namespace
	fmt.Println(installExt.Spec.Helm.Namespace)

	releaseName := installExt.Spec.Helm.Name
	chartVersion := installExt.Spec.Helm.Version
	values, err := helmClient.ConvertHelmValues(installExt.Spec.Helm.Values)
	if err != nil {
		log.Error(err, "failed to convert Helm values")
		return ctrl.Result{}, err
	}

	chart := ""

	switch installExt.Spec.Helm.Type {
	case "oci":
		chart = "oci://" + installExt.Spec.Helm.URL
	default:
	}

	settings := cli.New()
	settings.SetNamespace(namespace)

	helm, err := helmClient.New(settings)
	if err != nil {
		log.Error(err, "failed to create Helm client")
		return ctrl.Result{}, err
	}

	rancherMgr := rancher.NewManager(r.Client, r.Scheme)

	if !installExt.ObjectMeta.DeletionTimestamp.IsZero() {
		if err := r.handleDeletion(
			ctx,
			&installExt,
			helm,
			rancherMgr,
			releaseName,
		); err != nil {
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

	err = helm.EnsureRelease(ctx, helmClient.ReleaseSpec{
		Name:      releaseName,
		Namespace: namespace,
		ChartRef:  chart,
		Version:   chartVersion,
		Values:    values,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	svc, err := kubernetes.ServiceForHelmRelease(ctx, r.Client, namespace, releaseName)
	if err != nil {
		msg := fmt.Sprintf("Error to fetch services")
		log.Info(msg)
	}

	svcName, svcNamespace, svcPort, err := installaiextension.ServiceEndpoint(svc)
	if err != nil {
		msg := fmt.Sprintf("Error to fetch svc info")
		log.Info(msg)
	}

	svcURL = fmt.Sprintf("http://%s.%s:%d", svcName, svcNamespace, svcPort)

	if err := rancherMgr.Ensure(ctx, &installExt, svcURL); err != nil {
		return ctrl.Result{}, err
	}

	var latest aiplatformv1alpha1.InstallAIExtension
	if err := r.Get(ctx, req.NamespacedName, &latest); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if latest.Status == (aiplatformv1alpha1.InstallAIExtensionStatus{}) {
		latest.Status = aiplatformv1alpha1.InstallAIExtensionStatus{}
	}

	latest.Status.Phase = "Installed"
	latest.Status.Message = fmt.Sprintf(
		"Extension %s installed",
		latest.Spec.Extension.Name,
	)

	if err := r.Status().Update(ctx, &latest); err != nil {
		log.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *InstallAIExtensionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiplatformv1alpha1.InstallAIExtension{}).
		Named("InstallAIExtension").
		Complete(r)
}
