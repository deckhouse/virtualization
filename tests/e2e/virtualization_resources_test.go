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

package e2e

import (
	"encoding/json"
	"fmt"
	"strings"

	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"
)

var _ = Describe("Virtualization resources", Ordered, ContinueOnFailure, func() {
	checkDefaultStorageClass := func() {
		storageClass := kc.Resource("sc")
		res := kubectl.List(storageClass, kc.GetOptions{Output: "json"})
		Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

		defaultStorageClassFlag := false
		var scList storagev1.StorageClassList
		err := json.Unmarshal([]byte(res.StdOut()), &scList)
		Expect(err).NotTo(HaveOccurred(), err)

		for _, sc := range scList.Items {
			isDefault, ok := sc.Annotations["storageclass.kubernetes.io/is-default-class"]
			if ok && isDefault == "true" {
				defaultStorageClassFlag = true
				break
			}
		}
		Expect(defaultStorageClassFlag).To(Equal(true), "error: missing default storage class in the cluster")
	}

	checkPhase := func(resource, phase string) {
		resourceType := kc.Resource(resource)
		jsonPath := fmt.Sprintf("'jsonpath={.status.phase}=%s'", phase)

		res := kubectl.List(resourceType, kc.GetOptions{
			Namespace: conf.Namespace,
			Output:    "jsonpath='{.items[*].metadata.name}'",
		})
		Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

		resources := strings.Split(res.StdOut(), " ")
		waitResult := kubectl.WaitResources(resourceType, resources, kc.WaitOptions{
			Namespace: conf.Namespace,
			For:       jsonPath,
			Timeout:   600,
		})
		Expect(waitResult.WasSuccess()).To(Equal(true), waitResult.StdErr())
	}

	BeforeAll(func() {
		checkDefaultStorageClass()
	})

	Context("Virtualization resources", func() {
		When("Resources applied", func() {
			It("Result must have no error", func() {
				res := kubectl.Kustomize(conf.VirtualizationResources, kc.KustomizeOptions{})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
			})
		})
	})

	Context("Virtual images", func() {
		When("VI applied", func() {
			It(fmt.Sprintf("Phase should be %s", PhaseReady), func() {
				checkPhase("vi", PhaseReady)
			})
		})
	})

	Context("Disks", func() {
		When("VD applied", func() {
			It(fmt.Sprintf("Phase should be %s", PhaseReady), func() {
				checkPhase("vd", PhaseReady)
			})
		})
	})

	Context("Virtual machines", func() {
		When("VM applied", func() {
			It(fmt.Sprintf("Phase should be %s", PhaseRunning), func() {
				checkPhase("vm", PhaseRunning)
			})
		})
	})

	Context("Virtual machines IP addresses", func() {
		When("VMIP applied", func() {
			It(fmt.Sprintf("Phase should be %s", PhaseBound), func() {
				checkPhase("vmip", PhaseBound)
			})
		})
	})
})
