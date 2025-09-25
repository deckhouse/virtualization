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

package util

import (
	"fmt"

	. "github.com/onsi/gomega"

	dv1alpha2 "github.com/deckhouse/virtualization/tests/e2e/api/deckhouse/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	"github.com/deckhouse/virtualization/tests/e2e/helper"
)

// Deprecated: Should be deleted
func PrepareProject(testData string) {
	kustomize := &config.Kustomize{}

	kustomization := fmt.Sprintf("%s/%s", testData, "kustomization.yaml")
	ns, err := kustomize.GetNamespace(kustomization)
	Expect(err).NotTo(HaveOccurred(), err)
	project := dv1alpha2.Project{}
	projectFilePath := fmt.Sprintf("%s/project/project.yaml", testData)

	err = helper.UnmarshalResource(projectFilePath, &project)
	Expect(err).NotTo(HaveOccurred(), "cannot get project from file: %s\nstderr: %s", projectFilePath, err)

	namePrefix, err := framework.NewFramework("").GetNamePrefix()
	Expect(err).NotTo(HaveOccurred(), "cannot get name prefix\nstderr: %s", err)

	project.Name = ns

	if project.Labels == nil {
		project.SetLabels(make(map[string]string))
	}
	project.Labels["id"] = namePrefix

	err = helper.WriteYamlObject(projectFilePath, &project)
	Expect(err).NotTo(HaveOccurred(), "cannot update project with id and labels: %s\nstderr: %s", projectFilePath, err)
}
