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
	"fmt"
	"strings"

	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Virtualization resources", Ordered, ContinueOnFailure, func() {
	BeforeAll(func() {
		By("Setup virtual machine custom IP address")
		var (
			err          error
			unassignedIP string
		)
		filePath := fmt.Sprintf("%s/%s", conf.VirtualizationResources, "vm/overlays/custom-ip/vmip.yaml")
		unassignedIP, err = FindCustomSubnetUnassignedIP(conf.CustomSubnet)
		Expect(err).NotTo(HaveOccurred())
		err = SetCustomIPAddress(filePath, unassignedIP)
		Expect(err).NotTo(HaveOccurred())
	})
	checkPhase := func(resource, phase string) {
		resourceType := kc.Resource(resource)
		jsonPath := fmt.Sprintf("'jsonpath={.status.phase}=%s'", phase)

		res := kubectl.List(resourceType, kc.GetOptions{
			Namespace: conf.Namespace,
			Output:    "jsonpath='{.items[*].metadata.name}'",
		})
		Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

		resources := strings.Split(res.StdOut(), " ")
		waitOpts := kc.WaitOptions{
			Namespace: conf.Namespace,
			For:       jsonPath,
			Timeout:   600,
		}
		waitResult := kubectl.WaitResources(resourceType, waitOpts, resources...)
		Expect(waitResult.WasSuccess()).To(Equal(true), waitResult.StdErr())
	}

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

	Context("Virtual machines IP addresses", func() {
		When("VMIP applied", func() {
			It(fmt.Sprintf("Phase should be %s", PhaseBound), func() {
				checkPhase("vmip", PhaseBound)
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
})
