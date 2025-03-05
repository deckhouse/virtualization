/*
Copyright 2024 Flant JSC

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

package config

import (
	"fmt"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	. "github.com/deckhouse/virtualization/tests/e2e/helper"
)

var kustomize *Kustomize

func PrepareProject(testData string) {
	kustomization := fmt.Sprintf("%s/%s", testData, "kustomization.yaml")
	ns, err := kustomize.GetNamespace(kustomization)
	project := Project{}
	projectFilePath := fmt.Sprintf("%s/project/project.yaml", testData)

	err = UnmarshalResource(projectFilePath, &project)
	Expect(err).NotTo(HaveOccurred(), "cannot get project from file: %s\nstderr: %s", projectFilePath, err)

	namePrefix, err := GetNamePrefix()
	if err != nil {
	}

	project.Name = ns

	if project.Labels == nil {
		project.SetLabels(make(map[string]string))
	}
	project.Labels["id"] = namePrefix

	err = WriteYamlObject(projectFilePath, &project)
	Expect(err).NotTo(HaveOccurred(), "cannot update project with id and labels: %s\nstderr: %s", projectFilePath, err)
}

type Project struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ProjectSpec `json:"spec,omitempty"`
}

func (p *Project) DeepCopyObject() runtime.Object {
	return p.DeepCopy()
}

func (p *Project) DeepCopy() *Project {
	if p == nil {
		return nil
	}
	newObj := Project{}
	p.DeepCopyInto(&newObj)
	return &newObj
}

func (p *Project) DeepCopyInto(newObj *Project) {
	*newObj = *p
	newObj.TypeMeta = p.TypeMeta
	p.ObjectMeta.DeepCopyInto(&newObj.ObjectMeta)
	p.Spec.DeepCopyInto(&newObj.Spec)
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

func (p *ProjectSpec) DeepCopy() *ProjectSpec {
	if p == nil {
		return nil
	}
	newObj := new(ProjectSpec)
	p.DeepCopyInto(newObj)
	return newObj
}

func (p *ProjectSpec) DeepCopyInto(newObj *ProjectSpec) {
	*newObj = *p
	newObj.Description = p.Description
	newObj.ProjectTemplateName = p.ProjectTemplateName
	newObj.Parameters = make(map[string]interface{})
	for key, value := range p.Parameters {
		newObj.Parameters[key] = value
	}
}
