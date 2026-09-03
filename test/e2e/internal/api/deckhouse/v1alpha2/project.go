/*
Copyright 2025 Flant JSC

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

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectStateDeployed is the value of Project.Status.State once the Project and
// every resource it renders have been successfully applied.
const ProjectStateDeployed = "Deployed"

type Project struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ProjectSpec   `json:"spec,omitempty"`
	Status            ProjectStatus `json:"status,omitempty"`
}

// ProjectStatus is a partial mirror of the upstream deckhouse.io/v1alpha2 Project
// status: only the fields the e2e tests need are modelled here (currently the
// aggregate State, e.g. "Deployed").
type ProjectStatus struct {
	// State is the aggregate state of the Project ("Deployed", "Error", ...).
	State string `json:"state,omitempty"`
}

type ProjectSpec struct {
	// Description of the Project
	Description string `json:"description,omitempty"`

	// Name of ProjectTemplate to use to create Project
	ProjectTemplateName string `json:"projectTemplateName,omitempty"`

	// Values for resource templates from ProjectTemplate
	// in helm values format that map to the open-api specification
	// from the ValuesSchema ProjectTemplate field
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Project `json:"items"`
}
