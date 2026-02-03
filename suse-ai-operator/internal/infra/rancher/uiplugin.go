package rancher

import (
	"context"
	"fmt"

	"github.com/SUSE/suse-ai-operator/api/v1alpha1"
	logging "github.com/SUSE/suse-ai-operator/internal/logging"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (m *Manager) ensureUIPlugin(
	ctx context.Context,
	ext *v1alpha1.InstallAIExtension,
	svcURL string,
) error {
	log := logging.FromContext(ctx, "rancher.uiplugin").
		WithValues(
			logging.KeyExtension, ext.Spec.Extension.Name,
			logging.KeyVersion, ext.Spec.Extension.Version,
		)

	ui := &unstructured.Unstructured{}
	ui.SetAPIVersion("catalog.cattle.io/v1")
	ui.SetKind("UIPlugin")
	ui.SetName(ext.Spec.Extension.Name)

	namespace := ext.Spec.Extension.Namespace
	if namespace == "" {
		namespace = "cattle-ui-plugin-system"
	}
	ui.SetNamespace(namespace)

	log.Info(
		"Ensuring UIPlugin",
		"namespace", namespace,
	)

	_, err := ctrl.CreateOrUpdate(ctx, m.client, ui, func() error {
		if err := unstructured.SetNestedField(ui.Object, ext.Spec.Extension.Name, "spec", "plugin", "name"); err != nil {
			return err
		}
		if err := unstructured.SetNestedField(ui.Object, ext.Spec.Extension.Version, "spec", "plugin", "version"); err != nil {
			return err
		}
		pluginEndpoint := fmt.Sprintf("%s/plugin/%s-%s", svcURL, ext.Spec.Extension.Name, ext.Spec.Extension.Version)
		if err := unstructured.SetNestedField(ui.Object, pluginEndpoint, "spec", "plugin", "endpoint"); err != nil {
			return err
		}

		logging.Trace(log).Info(
			"Configuring UIPlugin spec",
			"endpoint", pluginEndpoint,
		)

		metadata := ext.Spec.Extension.Metadata
		if metadata == nil {
			metadata = map[string]string{}
		}

		metadata, err := buildExtensionMetadata(
			ctx,
			m.indexCache,
			svcURL,
			ext.Spec.Extension.Name,
			ext.Spec.Extension.Version,
			metadata,
		)

		if err != nil {
			return err
		}

		return unstructured.SetNestedStringMap(ui.Object, metadata, "spec", "plugin", "metadata")
	})
	if err != nil {
		return err
	}

	logging.Debug(log).Info("UIPlugin ensured")
	return nil
}

func (m *Manager) deleteUIPlugin(
	ctx context.Context,
	ext *v1alpha1.InstallAIExtension,
) error {
	log := logging.FromContext(ctx, "rancher.uiplugin").
		WithValues(
			logging.KeyExtension, ext.Spec.Extension.Name,
			logging.KeyVersion, ext.Spec.Extension.Version,
		)

	namespace := ext.Spec.Extension.Namespace
	if namespace == "" {
		namespace = "cattle-ui-plugin-system"
	}

	log.Info(
		"Deleting UIPlugin",
		logging.KeyNamespace, namespace,
	)

	ui := &unstructured.Unstructured{}
	ui.SetAPIVersion("catalog.cattle.io/v1")
	ui.SetKind("UIPlugin")
	ui.SetName(ext.Spec.Extension.Name)
	ui.SetNamespace(namespace)

	err := m.client.Delete(ctx, ui)
	if client.IgnoreNotFound(err) == nil {
		logging.Debug(log).Info("UIPlugin already deleted or not found")
		return nil
	}

	if err != nil {
		log.Error(err, "Failed to delete UIPlugin")
		return err
	}

	log.Info("UIPlugin deleted")
	return nil
}
