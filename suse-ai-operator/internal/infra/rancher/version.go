package rancher

import (
	"context"
	"fmt"

	logging "github.com/SUSE/suse-ai-operator/internal/logging"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func (m *Manager) getVersion(ctx context.Context) (string, error) {
	log := logging.FromContext(ctx, "rancher.version")
	logging.Debug(log).Info("Reading Rancher server-version setting")

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("management.cattle.io/v3")
	obj.SetKind("Setting")

	err := m.client.Get(ctx, types.NamespacedName{
		Name: "server-version",
	}, obj)
	if err != nil {
		return "", fmt.Errorf("failed to get server-version setting: %w", err)
	}

	value, found, err := unstructured.NestedString(obj.Object, "value")
	if err != nil {
		return "", fmt.Errorf("failed parsing server-version value: %w", err)
	}
	if !found {
		return "", fmt.Errorf("server-version setting is missing .value")
	}

	return value, nil
}
