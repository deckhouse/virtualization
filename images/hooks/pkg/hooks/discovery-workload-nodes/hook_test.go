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
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	"github.com/deckhouse/virtualization/hooks/pkg/settings"
)

func TestDiscoveryWorkloadNodes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DiscoveryWorkloadNodes Suite")
}

type fakeKubernetesClient struct {
	pkg.KubernetesClient
	get func(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object) error
}

func (f *fakeKubernetesClient) Get(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object, _ ...ctrlclient.GetOption) error {
	return f.get(ctx, key, obj)
}

var _ = Describe("DiscoveryWorkloadNodes", func() {
	var (
		dc        *mock.DependencyContainerMock
		snapshots *mock.SnapshotsMock
		values    *mock.OutputPatchableValuesCollectorMock
	)

	newInput := func(withModuleConfig bool) *pkg.HookInput {
		dc.GetK8sClientMock.Return(&fakeKubernetesClient{get: func(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object) error {
			mc := obj.(*mcapi.ModuleConfig)
			if withModuleConfig {
				*mc = *settings.NewModuleConfigForTest(map[string]any{"dvcr": map[string]any{}, "virtualMachineCIDRs": []any{"10.0.0.0/24"}})
			} else {
				*mc = *settings.NewModuleConfigForTest(nil)
			}
			return nil
		}}, nil)
		return &pkg.HookInput{
			DC:        dc,
			Snapshots: snapshots,
			Values:    values,
			Logger:    log.NewNop(),
		}
	}

	BeforeEach(func() {
		dc = mock.NewDependencyContainerMock(GinkgoT())
		snapshots = mock.NewSnapshotsMock(GinkgoT())
		values = mock.NewPatchableValuesCollectorMock(GinkgoT())
		values.GetMock.When(settings.InternalValuesConfigCopyPath).Then(gjson.Result{})
	})

	AfterEach(func() {
		dc = nil
		snapshots = nil
		values = nil
	})

	It("should do nothing when module config is incomplete", func() {
		Expect(handleDiscoveryNodes(context.Background(), newInput(false))).To(Succeed())
	})

	It("should set node count when module config exists", func() {
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

		Expect(handleDiscoveryNodes(context.Background(), newInput(true))).To(Succeed())
		Expect(setCalls).To(Equal(1))
	})
})
