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

package discovery_workload_nodes

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
)

func TestDiscoveryWorkloadNodes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DiscoveryWorkloadNodes Suite")
}

var _ = Describe("DiscoveryWorkloadNodes", func() {
	var (
		snapshots *mock.SnapshotsMock
		values    *mock.OutputPatchableValuesCollectorMock
	)

	newInput := func() *pkg.HookInput {
		return &pkg.HookInput{
			Snapshots: snapshots,
			Values:    values,
			Logger:    log.NewNop(),
		}
	}

	BeforeEach(func() {
		snapshots = mock.NewSnapshotsMock(GinkgoT())
		values = mock.NewPatchableValuesCollectorMock(GinkgoT())
	})

	AfterEach(func() {
		snapshots = nil
		values = nil
	})

	It("should do nothing when copied module config is absent", func() {
		values.GetMock.When("virtualization.internal.moduleConfig").Then(gjson.Result{})
		Expect(handleDiscoveryNodes(context.Background(), newInput())).To(Succeed())
	})

	It("should set node count when copied module config exists", func() {
		values.GetMock.When("virtualization.internal.moduleConfig").Then(gjson.Parse(`{"dvcr":{},"virtualMachineCIDRs":["10.0.0.0/24"]}`))
		snapshots.GetMock.When(discoveryNodesSnapshot).Then([]pkg.Snapshot{
			mock.NewSnapshotMock(GinkgoT()),
			mock.NewSnapshotMock(GinkgoT()),
		})
		snapshots.GetMock.When(kubevirtConfigSnapshot).Then([]pkg.Snapshot{})

		setCalls := 0
		values.SetMock.Set(func(path string, v any) {
			setCalls++
			Expect(path).To(Equal(virtHandlerNodeCountPath))
			Expect(v).To(Equal(2))
		})

		Expect(handleDiscoveryNodes(context.Background(), newInput())).To(Succeed())
		Expect(setCalls).To(Equal(1))
	})
})
