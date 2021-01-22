/*
Copyright 2020 The Kubernetes Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Controller",type=string,JSONPath=`.spec.controller`
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// GatewayClass describes a class of Gateways available to the user
// for creating Gateway resources.
//
// GatewayClass is a Cluster level resource.
//
// Support: Core.
type GatewayClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec for this GatewayClass.
	Spec GatewayClassSpec `json:"spec,omitempty"`
	// Status of the GatewayClass.
	// +kubebuilder:default={conditions: {{type: "InvalidParameters", status: "Unknown", message: "Waiting for controller", reason: "Waiting", lastTransitionTime: "1970-01-01T00:00:00Z"}}}
	Status GatewayClassStatus `json:"status,omitempty"`
}

// GatewayClassSpec reflects the configuration of a class of Gateways.
type GatewayClassSpec struct {
	// Controller is a domain/path string that indicates the
	// controller that is managing Gateways of this class.
	//
	// Example: "acme.io/gateway-controller".
	//
	// This field is not mutable and cannot be empty.
	//
	// The format of this field is DOMAIN "/" PATH, where DOMAIN
	// and PATH are valid Kubernetes names
	// (https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names).
	//
	// Support: Core
	//
	// +kubebuilder:validation:MaxLength=253
	Controller string `json:"controller"`

	// ParametersRef is a controller-specific resource containing the
	// configuration parameters corresponding to this class. This is optional if
	// the controller does not require any additional configuration.
	//
	// Parameters resources are implementation specific custom resources. These
	// resources must be cluster-scoped.
	//
	// If the referent cannot be found, the GatewayClass's "InvalidParameters"
	// status condition will be true.
	//
	// Support: Custom
	//
	// +optional
	ParametersRef *LocalObjectReference `json:"parametersRef,omitempty"`
}

// GatewayClassConditionType is the type of status conditions. This
// type should be used with the GatewayClassStatus.Conditions field.
type GatewayClassConditionType string

const (
	// GatewayClassConditionStatusInvalidParameters indicates the
	// validity of the Parameters set for a given controller. This
	// will initially start off as "Unknown".
	GatewayClassConditionStatusInvalidParameters GatewayClassConditionType = "InvalidParameters"

	// GatewayClassFinalizerGatewaysExist should be added as a finalizer to the
	// GatewayClass whenever there are provisioned Gateways using a GatewayClass.
	GatewayClassFinalizerGatewaysExist = "gateway-exists-finalizer.networking.x-k8s.io"
)

// GatewayClassStatus is the current status for the GatewayClass.
type GatewayClassStatus struct {
	// Conditions is the current status from the controller for
	// this GatewayClass.
	//
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	// +kubebuilder:default={{type: "InvalidParameters", status: "Unknown", message: "Waiting for controller", reason: "Waiting", lastTransitionTime: "1970-01-01T00:00:00Z"}}
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

// GatewayClassList contains a list of GatewayClass
type GatewayClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayClass `json:"items"`
}
