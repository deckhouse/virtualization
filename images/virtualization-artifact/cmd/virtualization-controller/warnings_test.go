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

package main

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type recordedWarnings struct{ texts []string }

func (r *recordedWarnings) HandleWarningHeaderWithContext(_ context.Context, _ int, _, text string) {
	r.texts = append(r.texts, text)
}

var _ = DescribeTable("vmipDeprecationFilter",
	func(text string, passed bool) {
		recorded := &recordedWarnings{}
		filter := vmipDeprecationFilter{next: recorded}

		filter.HandleWarningHeaderWithContext(context.Background(), 299, "", text)

		if passed {
			Expect(recorded.texts).To(ConsistOf(text))
		} else {
			Expect(recorded.texts).To(BeEmpty())
		}
	},
	Entry("drops the VirtualMachineIPAddress deprecation",
		"VirtualMachineIPAddress is deprecated. Where the sdn module serves the Main network, leave virtualMachineIPAddressName unset or reference an IPAddress (network.deckhouse.io) in spec.networks[].ipAddressName of the Main network.", false),
	Entry("drops the VirtualMachineIPAddressLease deprecation",
		"VirtualMachineIPAddressLease is deprecated. The address of the Main network of a virtual machine is managed by the sdn module.", false),
	Entry("drops the spec.virtualMachineIPAddressName deprecation",
		"spec.virtualMachineIPAddressName is deprecated: the address of the Main network is managed by the IPAddress resources of the sdn module", false),
	Entry("passes a foreign warning",
		"metadata.finalizers: \"example\": prefer a domain-qualified finalizer name", true),
)
