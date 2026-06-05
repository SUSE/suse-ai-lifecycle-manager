package v1alpha1

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1beta1 "github.com/SUSE/suse-ai-operator/api/v1beta1"
)

// ConvertTo converts this InstallAIExtension (v1alpha1) to the Hub version (v1beta1).
func (src *InstallAIExtension) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*v1beta1.InstallAIExtension)
	if !ok {
		return fmt.Errorf("expected *v1beta1.InstallAIExtension, got %T", dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	dst.Spec.Extension = v1beta1.ExtensionSpec{
		Name:     src.Spec.Extension.Name,
		Version:  src.Spec.Extension.Version,
		Metadata: src.Spec.Extension.Metadata,
	}

	if src.Spec.Source != nil {
		switch {
		case src.Spec.Source.Git != nil:
			dst.Spec.Source.Git = &v1beta1.GitSpec{
				Repo:   src.Spec.Source.Git.Repo,
				Branch: src.Spec.Source.Git.Branch,
			}
		case src.Spec.Source.Helm != nil:
			dst.Spec.Source.Helm = &v1beta1.HelmSpec{
				Name:    src.Spec.Source.Helm.Name,
				URL:     src.Spec.Source.Helm.URL,
				Version: src.Spec.Source.Helm.Version,
				Values:  src.Spec.Source.Helm.Values,
			}
		}
	} else if src.Spec.Helm != nil {
		dst.Spec.Source.Helm = &v1beta1.HelmSpec{
			Name:    src.Spec.Helm.Name,
			URL:     src.Spec.Helm.URL,
			Version: src.Spec.Helm.Version,
			Values:  src.Spec.Helm.Values,
		}
	}

	// Status — same shape in both versions
	dst.Status.Phase = src.Status.Phase
	dst.Status.Message = src.Status.Message
	dst.Status.ResolvedVersion = src.Status.ResolvedVersion

	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha1).
func (dst *InstallAIExtension) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*v1beta1.InstallAIExtension)
	if !ok {
		return fmt.Errorf("expected *v1beta1.InstallAIExtension, got %T", srcRaw)
	}

	// ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Spec.Extension
	dst.Spec.Extension = ExtensionSpec{
		Name:     src.Spec.Extension.Name,
		Version:  src.Spec.Extension.Version,
		Metadata: src.Spec.Extension.Metadata,
	}

	// Spec.Source -> both helm (back-compat) and source (new)
	dst.Spec.Source = &SourceSpec{}
	if src.Spec.Source.Helm != nil {
		helmSpec := &HelmSpec{
			Name:    src.Spec.Source.Helm.Name,
			URL:     src.Spec.Source.Helm.URL,
			Version: src.Spec.Source.Helm.Version,
			Values:  src.Spec.Source.Helm.Values,
		}
		dst.Spec.Helm = helmSpec
		dst.Spec.Source.Helm = helmSpec
	}
	if src.Spec.Source.Git != nil {
		dst.Spec.Source.Git = &GitSpec{
			Repo:   src.Spec.Source.Git.Repo,
			Branch: src.Spec.Source.Git.Branch,
		}
	}
	// Clean up empty source
	if dst.Spec.Source.Helm == nil && dst.Spec.Source.Git == nil {
		dst.Spec.Source = nil
	}

	// Status — same shape in both versions
	dst.Status.Phase = src.Status.Phase
	dst.Status.Message = src.Status.Message
	dst.Status.ResolvedVersion = src.Status.ResolvedVersion

	return nil
}
