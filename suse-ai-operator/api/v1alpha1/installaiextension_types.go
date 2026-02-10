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

package v1alpha1

import (
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// InstallAIExtensionSpec defines the desired state of InstallAIExtension
type InstallAIExtensionSpec struct {
	Helm *HelmSpec `json:"helm,omitempty"`

	// +kubebuilder:validation:Required
	Extension ExtensionSpec `json:"extension"`
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

type ExtensionSpec struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// +kubebuilder:validation:MinLength=1
	Version  string            `json:"version"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// InstallAIExtensionStatus defines the observed state of InstallAIExtension.
type InstallAIExtensionStatus struct {
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// InstallAIExtension is the Schema for the installaiextensions API
type InstallAIExtension struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of InstallAIExtension
	// +required
	Spec InstallAIExtensionSpec `json:"spec"`

	// status defines the observed state of InstallAIExtension
	// +optional
	Status InstallAIExtensionStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// InstallAIExtensionList contains a list of InstallAIExtension
type InstallAIExtensionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InstallAIExtension `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InstallAIExtension{}, &InstallAIExtensionList{})
}
