package installaiextension

import (
	"fmt"

	"github.com/SUSE/suse-ai-operator/api/v1alpha1"
)

type InstallSource string

const (
	SourceHelm InstallSource = "helm"
	SourceRepo InstallSource = "repo"
)

func ValidateSpec(spec v1alpha1.InstallAIExtensionSpec) (InstallSource, error) {
	hasHelm := spec.Helm != nil
	hasRepo := spec.Repo != nil

	switch {
	case hasHelm && hasRepo:
		return "", fmt.Errorf("only one of helm or repo may be set")
	case hasHelm:
		return SourceHelm, nil
	case hasRepo:
		return SourceRepo, nil
	default:
		return "", fmt.Errorf("either helm or repo must be set")
	}
}
