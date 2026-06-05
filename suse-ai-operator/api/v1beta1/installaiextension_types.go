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

package v1beta1

import (
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	VersionPolicyManaged   = "managed"
	VersionPolicyUnmanaged = "unmanaged"
	SourceTypeHelm         = "helm"
	SourceTypeGit          = "git"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// InstallAIExtensionSpec defines the desired state of InstallAIExtension
// +kubebuilder:validation:XValidation:rule="!has(self.source.helm) || self.extension.versionPolicy != 'unmanaged'",message="versionPolicy \"unmanaged\" is only supported with git sources"
// +kubebuilder:validation:XValidation:rule=”!has(self.source.helm) || self.extension.version != ''”,message=”spec.extension.version is required for helm sources”
// +kubebuilder:validation:XValidation:rule=”!has(self.source.helm) || self.source.helm.version != ''”,message=”spec.source.helm.version is required”
type InstallAIExtensionSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// +kubebuilder:validation:Required
	Source SourceSpec `json:"source"`

	// +kubebuilder:validation:Required
	Extension ExtensionSpec `json:"extension"`
}

// +kubebuilder:validation:XValidation:rule="has(self.helm) || has(self.git)",message="source must specify either helm or git"
// +kubebuilder:validation:XValidation:rule="!(has(self.helm) && has(self.git))",message="source must specify either helm or git, not both"
type SourceSpec struct {
	// Helm chart source
	// +optional
	Helm *HelmSpec `json:"helm,omitempty"`

	// Git repository source
	// +optional
	Git *GitSpec `json:"git,omitempty"`
}

type HelmSpec struct {
	Name string `json:"name"`
	// URL of the Helm repository or OCI registry.
	// Examples:
	//   oci://ghcr.io/my-org/charts
	//   https://charts.example.com
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^(oci://|https?://).+`
	URL     string                 `json:"url"`
	Version string                 `json:"version"`
	Values  map[string]apixv1.JSON `json:"values,omitempty"`
}

type GitSpec struct {
	// GitHub repository URL.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^https?://github\.com/.+$`
	Repo string `json:"repo"`

	// Git branch to use
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Branch string `json:"branch"`
}

type ExtensionSpec struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// +optional
	Version string `json:"version,omitempty"`

	// Version management policy (git sources only).
	// "managed" uses the explicit version if set, or resolves latest if empty.
	// "unmanaged" installs once via helm release, then user manages from Rancher UI.
	// +kubebuilder:validation:Enum=managed;unmanaged
	// +kubebuilder:default=managed
	// +optional
	VersionPolicy string `json:"versionPolicy,omitempty"`

	Metadata map[string]string `json:"metadata,omitempty"`
}

// InstallAIExtensionStatus defines the observed state of InstallAIExtension.
type InstallAIExtensionStatus struct {
	Phase           string `json:"phase,omitempty"`
	Message         string `json:"message,omitempty"`
	ResolvedVersion string `json:"resolvedVersion,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=iae
// +kubebuilder:storageversion

// InstallAIExtension is the Schema for the installaiextensions API
type InstallAIExtension struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of InstallAIExtension
	// +required
	Spec InstallAIExtensionSpec `json:"spec"`

	// status defines the observed state of InstallAIExtension
	// +optional
	Status InstallAIExtensionStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// InstallAIExtensionList contains a list of InstallAIExtension
type InstallAIExtensionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []InstallAIExtension `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InstallAIExtension{}, &InstallAIExtensionList{})
}
