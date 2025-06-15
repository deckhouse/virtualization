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

package main

import (
	"context"
	"testing"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDiscoveryVirthandlerNodes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Discovery virt-handler nodes Suite")
}

var _ = Describe("Discovery virt-handler nodes", func() {
	var (
		snapshots *mock.SnapshotsMock
		values    *mock.PatchableValuesCollectorMock
		input     *pkg.HookInput
	)

	BeforeEach(func() {
		snapshots = mock.NewSnapshotsMock(GinkgoT())
		values = mock.NewPatchableValuesCollectorMock(GinkgoT())

		input = &pkg.HookInput{
			Values:    values,
			Snapshots: snapshots,
		}
	})

	Context("Empty cluster", func() {
		It("Hook must execute successfully", func() {
			snapshots.GetMock.When(nodesSnapshot).Then(
				[]pkg.Snapshot{},
			)
			values.SetMock.When(virtHandlerNodeCountPath, 0)
			err := handleDiscoveryVirtHandlerNodes(context.Background(), input)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	Context("Two nodes should be discovered.", func() {
		It("Hook must execute successfully", func() {

			snapshots.GetMock.When(nodesSnapshot).Then(
				[]pkg.Snapshot{
					mock.NewSnapshotMock(GinkgoT()).StringMock.Return("n1"),
					mock.NewSnapshotMock(GinkgoT()).StringMock.Return("n2"),
				},
			)

			values.SetMock.When(virtHandlerNodeCountPath, 2)
			err := handleDiscoveryVirtHandlerNodes(context.Background(), input)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})

})
