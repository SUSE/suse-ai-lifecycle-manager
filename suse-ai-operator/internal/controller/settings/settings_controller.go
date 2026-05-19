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

package settings

import (
	"context"

	aiplatformv1alpha1 "github.com/SUSE/suse-ai-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SettingsReconciler reconciles a Settings object.
type SettingsReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ai-platform.suse.com,resources=settings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ai-platform.suse.com,resources=settings/status,verbs=get;update;patch

func (r *SettingsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	var s aiplatformv1alpha1.Settings
	if err := r.Get(ctx, req.NamespacedName, &s); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	now := metav1.Now()
	s.Status.LastApplied = &now
	s.Status.ObservedGeneration = s.Generation

	if err := r.Status().Update(ctx, &s); err != nil {
		l.Error(err, "failed to update settings status")
		return ctrl.Result{}, err
	}

	l.Info("reconciled settings", "name", s.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller with the Manager.
func (r *SettingsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiplatformv1alpha1.Settings{}).
		Complete(r)
}
