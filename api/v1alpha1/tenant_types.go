/*
Copyright 2026.

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

// TenantPlan defines the billing plan for a tenant.
// +kubebuilder:validation:Enum=free;starter;pro;enterprise
type TenantPlan string

const (
	PlanFree       TenantPlan = "free"
	PlanStarter    TenantPlan = "starter"
	PlanPro        TenantPlan = "pro"
	PlanEnterprise TenantPlan = "enterprise"
)

// TenantPhase describes the lifecycle phase of the tenant.
// +kubebuilder:validation:Enum=Pending;Active;Deleting;Failed
type TenantPhase string

const (
	TenantPhasePending  TenantPhase = "Pending"
	TenantPhaseActive   TenantPhase = "Active"
	TenantPhaseDeleting TenantPhase = "Deleting"
	TenantPhaseFailed   TenantPhase = "Failed"
)

// ResourceLimits defines optional resource limit overrides for a tenant.
type ResourceLimits struct {
	// Maximum total CPU across all apps.
	// +optional
	MaxCPU string `json:"maxCpu,omitempty"`
	// Maximum total memory across all apps.
	// +optional
	MaxMemory string `json:"maxMemory,omitempty"`
}

// TenantSpec defines the desired state of Tenant.
type TenantSpec struct {
	// Human-readable display name for the tenant.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	DisplayName string `json:"displayName"`

	// Email of the tenant owner.
	// +optional
	OwnerEmail string `json:"ownerEmail,omitempty"`

	// Billing plan for the tenant.
	// +kubebuilder:default=free
	Plan TenantPlan `json:"plan,omitempty"`

	// Optional resource limit overrides. If not set, defaults based on plan are used.
	// +optional
	ResourceLimits *ResourceLimits `json:"resourceLimits,omitempty"`

	// Maximum number of apps allowed for this tenant.
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	MaxApps int32 `json:"maxApps,omitempty"`

	// Whether to enforce network isolation between tenants.
	// +kubebuilder:default=true
	NetworkIsolation *bool `json:"networkIsolation,omitempty"`
}

// TenantStatus defines the observed state of Tenant.
type TenantStatus struct {
	// Current lifecycle phase of the tenant.
	// +optional
	Phase TenantPhase `json:"phase,omitempty"`

	// The Kubernetes namespace assigned to this tenant.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Number of currently active apps in this tenant.
	// +optional
	ActiveApps int32 `json:"activeApps,omitempty"`

	// Standard conditions for the tenant.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=tnt
// +kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`
// +kubebuilder:printcolumn:name="Plan",type=string,JSONPath=`.spec.plan`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.status.namespace`
// +kubebuilder:printcolumn:name="Apps",type=integer,JSONPath=`.status.activeApps`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Tenant is the Schema for the tenants API.
type Tenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantSpec   `json:"spec,omitempty"`
	Status TenantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TenantList contains a list of Tenant.
type TenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tenant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tenant{}, &TenantList{})
}

// IsNetworkIsolationEnabled returns whether network isolation is enabled for this tenant.
func (t *Tenant) IsNetworkIsolationEnabled() bool {
	if t.Spec.NetworkIsolation == nil {
		return true
	}
	return *t.Spec.NetworkIsolation
}

// GetNamespaceName returns the namespace name for this tenant.
func (t *Tenant) GetNamespaceName() string {
	return "tenant-" + t.Name
}
