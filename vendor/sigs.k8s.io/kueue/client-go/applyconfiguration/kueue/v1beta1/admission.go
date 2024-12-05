/*
Copyright The Kubernetes Authors.

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
// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1beta1

import (
	v1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

// AdmissionApplyConfiguration represents a declarative configuration of the Admission type for use
// with apply.
type AdmissionApplyConfiguration struct {
	ClusterQueue      *v1beta1.ClusterQueueReference       `json:"clusterQueue,omitempty"`
	PodSetAssignments []PodSetAssignmentApplyConfiguration `json:"podSetAssignments,omitempty"`
}

// AdmissionApplyConfiguration constructs a declarative configuration of the Admission type for use with
// apply.
func Admission() *AdmissionApplyConfiguration {
	return &AdmissionApplyConfiguration{}
}

// WithClusterQueue sets the ClusterQueue field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ClusterQueue field is set to the value of the last call.
func (b *AdmissionApplyConfiguration) WithClusterQueue(value v1beta1.ClusterQueueReference) *AdmissionApplyConfiguration {
	b.ClusterQueue = &value
	return b
}

// WithPodSetAssignments adds the given value to the PodSetAssignments field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, values provided by each call will be appended to the PodSetAssignments field.
func (b *AdmissionApplyConfiguration) WithPodSetAssignments(values ...*PodSetAssignmentApplyConfiguration) *AdmissionApplyConfiguration {
	for i := range values {
		if values[i] == nil {
			panic("nil value passed to WithPodSetAssignments")
		}
		b.PodSetAssignments = append(b.PodSetAssignments, *values[i])
	}
	return b
}