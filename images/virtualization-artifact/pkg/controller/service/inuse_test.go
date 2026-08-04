/*
Copyright 2026 Flant JSC

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

package service

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("InUseByVirtualMachinesMessage", func() {
	It("falls back to an unnamed VirtualMachine when no name is known", func() {
		Expect(InUseByVirtualMachinesMessage("ClusterVirtualImage", nil)).
			To(Equal("The ClusterVirtualImage is in use by a VirtualMachine."))
	})

	It("names the single VirtualMachine and how to release the resource", func() {
		Expect(InUseByVirtualMachinesMessage("VirtualImage", []string{"vm-a"})).
			To(Equal(`The VirtualImage is in use by the VirtualMachine "vm-a"; detach it or stop the VirtualMachine to release the VirtualImage.`))
	})

	It("reports the count and quotes every VirtualMachine name", func() {
		Expect(InUseByVirtualMachinesMessage("VirtualDisk", []string{"vm-a", "vm-b"})).
			To(Equal(`The VirtualDisk is in use by 2 VirtualMachines; detach it or stop them to release the VirtualDisk. In use by: "vm-a", "vm-b".`))
	})

	It("keeps the count and the hint ahead of the unbounded list of names", func() {
		names := make([]string, 0, 12)
		for i := 0; i < 12; i++ {
			names = append(names, fmt.Sprintf("vm-%02d", i))
		}

		msg := InUseByVirtualMachinesMessage("ClusterVirtualImage", names)
		Expect(msg).To(HavePrefix("The ClusterVirtualImage is in use by 12 VirtualMachines; detach it or stop them to release the ClusterVirtualImage. In use by: "))
		Expect(msg).To(HaveSuffix(`"vm-11".`))
	})

	It("bounds a ballooning message built from many VirtualMachines", func() {
		names := make([]string, 0, 500)
		for i := 0; i < 500; i++ {
			names = append(names, fmt.Sprintf("namespace/virtual-machine-%03d", i))
		}

		msg := InUseByVirtualMachinesMessage("ClusterVirtualImage", names)
		Expect(len([]rune(msg))).To(BeNumerically("<=", maxConditionMessageLength))
		Expect(msg).To(HaveSuffix(" (truncated)."))
		// The truncation neither swallows the terminating period nor splits a quoted name.
		Expect(strings.Count(msg, `"`)%2).To(Equal(0), "quotes must stay balanced: %s", msg)
		// The count and the actionable hint survive the truncation.
		Expect(msg).To(ContainSubstring("is in use by 500 VirtualMachines; detach it or stop them to release the ClusterVirtualImage"))
	})
})
