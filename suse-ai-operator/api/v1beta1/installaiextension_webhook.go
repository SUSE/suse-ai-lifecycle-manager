package v1beta1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:object:generate=false
type InstallAIExtensionCustomValidator struct {
	Client client.Reader
}

var _ admission.CustomValidator = &InstallAIExtensionCustomValidator{}

func (v *InstallAIExtensionCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	ext, ok := obj.(*InstallAIExtension)
	if !ok {
		return nil, fmt.Errorf("expected *InstallAIExtension, got %T", obj)
	}
	return nil, v.validateExtensionNameUniqueness(ctx, ext)
}

func (v *InstallAIExtensionCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	ext, ok := newObj.(*InstallAIExtension)
	if !ok {
		return nil, fmt.Errorf("expected *InstallAIExtension, got %T", newObj)
	}
	oldExt, ok := oldObj.(*InstallAIExtension)
	if !ok {
		return nil, fmt.Errorf("expected *InstallAIExtension for oldObj, got %T", oldObj)
	}
	if oldExt.Spec.Extension.Name == ext.Spec.Extension.Name {
		return nil, nil
	}
	return nil, v.validateExtensionNameUniqueness(ctx, ext)
}

func (v *InstallAIExtensionCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *InstallAIExtensionCustomValidator) validateExtensionNameUniqueness(ctx context.Context, ext *InstallAIExtension) error {
	var list InstallAIExtensionList
	if err := v.Client.List(ctx, &list); err != nil {
		return fmt.Errorf("failed to list InstallAIExtension resources: %w", err)
	}
	for _, other := range list.Items {
		if other.Name == ext.Name {
			continue
		}
		if other.Spec.Extension.Name == ext.Spec.Extension.Name {
			return fmt.Errorf(
				"extension name %q is already used by InstallAIExtension %q",
				ext.Spec.Extension.Name,
				other.Name,
			)
		}
	}
	return nil
}
