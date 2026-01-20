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

package validate_module_config

import (
	"context"
	"encoding/json"
	"hooks/pkg/settings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"

	corev1 "k8s.io/api/core/v1"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
)

func TestValidateModuleConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Validate ModuleConfig Suite")
}

var _ = Describe("ModuleConfig validation hook", func() {
	const (
		defaultPodSubnet     = "10.111.0.0/16"
		defaultServiceSubnet = "10.222.0.0/16"
	)
	var (
		snapshots    *mock.SnapshotsMock
		values       *mock.OutputPatchableValuesCollectorMock
		configValues *mock.OutputPatchableValuesCollectorMock

		defaultNodeAddresses = []string{
			"192.168.0.1",
			"192.168.0.2",
			"192.168.0.3",
		}

		validVirtualMachineCIDRs = []interface{}{
			"10.33.0.0/24",
			"10.35.0.0/24",
		}

		virtualMachineCIDRsOverlapWithNodeAddresses = []interface{}{
			"192.168.0.0/24",
			"10.35.0.0/24",
		}
		virtualMachineCIDRsOverlapWithPodSubnet = []interface{}{
			"10.108.0.0/14",
			"10.35.0.0/24",
		}
		virtualMachineCIDRsOverlapWithServiceSubnet = []interface{}{
			"10.220.0.0/14",
			"10.35.0.0/24",
		}
	)

	prepareValues := func(podSubnet, serviceSubnet string) {
		values.GetMock.When(podSubnetCIDRPath).Then(gjson.Result{Type: gjson.String, Str: podSubnet})
		values.GetMock.When(serviceSubnetCIDRPath).Then(gjson.Result{Type: gjson.String, Str: serviceSubnet})
	}

	prepareConfigValues := func(cidrs []interface{}) {
		cfgValues := map[string]interface{}{
			"virtualMachineCIDRs": cidrs,
		}
		cfgValuesBytes, _ := json.Marshal(cfgValues)

		configValues.GetMock.When("virtualization").Then(gjson.ParseBytes(cfgValuesBytes))
	}

	prepareSnapshots := func(mc, nodes []pkg.Snapshot) {
		snapshots.GetMock.When(snapshotModuleConfig).Then(mc)
		snapshots.GetMock.When(snapshotNodes).Then(nodes)
	}

	newModuleConfigSnapshot := func(cidrs []interface{}) []pkg.Snapshot {
		return []pkg.Snapshot{
			mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) (err error) {
				GinkgoHelper()
				mc, ok := v.(*mcapi.ModuleConfig)
				Expect(ok).To(BeTrue())

				mc.SetName("virtualization")

				if cidrs != nil {
					mc.Spec.Settings = map[string]interface{}{
						"virtualMachineCIDRs": cidrs,
					}
				}

				return nil
			}),
		}
	}

	newNodesSnapshot := func(nodesAddresses []string) []pkg.Snapshot {
		snap := make([]pkg.Snapshot, 0)
		for _, nodeAddr := range nodesAddresses {
			nodeName := "node-" + nodeAddr
			nodeIP := nodeAddr
			snap = append(snap, mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) (err error) {
				GinkgoHelper()
				node, ok := v.(*corev1.Node)
				Expect(ok).To(BeTrue())

				node.SetName(nodeName)

				node.Status.Addresses = []corev1.NodeAddress{
					{
						Type:    "InternalIP",
						Address: nodeIP,
					},
				}

				return nil
			}))
		}
		return snap
	}

	newInput := func() *pkg.HookInput {
		return &pkg.HookInput{
			Snapshots:    snapshots,
			Values:       values,
			ConfigValues: configValues,
			Logger:       log.NewNop(),
		}
	}

	BeforeEach(func() {
		snapshots = mock.NewSnapshotsMock(GinkgoT())
		values = mock.NewPatchableValuesCollectorMock(GinkgoT())
		configValues = mock.NewPatchableValuesCollectorMock(GinkgoT())
	})

	AfterEach(func() {
		snapshots = nil
		values = nil
		configValues = nil
	})

	It("Should copy moduleconfig settings into internal object if config is valid", func() {
		prepareValues(defaultPodSubnet, defaultServiceSubnet)
		prepareConfigValues(validVirtualMachineCIDRs)
		prepareSnapshots(
			newModuleConfigSnapshot(validVirtualMachineCIDRs),
			newNodesSnapshot(defaultNodeAddresses),
		)

		values.SetMock.Set(func(path string, v any) {
			switch path {
			case settings.InternalValuesConfigCopyPath:
				Expect(v).To(HaveKey("virtualMachineCIDRs"))
			case settings.InternalValuesConfigValidationPath:
				Fail("Should not set validation error")
			default:
				Fail("unexpected path")
			}
		})

		values.RemoveMock.Set(func(path string) {
			Expect(path).To(Equal(settings.InternalValuesConfigValidationPath), "should remove validation error")
		})

		Expect(Reconcile(context.Background(), newInput())).To(Succeed())
	})

	DescribeTable("Should set validation error if virtualMachineCIDRs overlap with other addresses in cluster", func(cidrs []interface{}) {
		prepareValues(defaultPodSubnet, defaultServiceSubnet)
		prepareSnapshots(
			newModuleConfigSnapshot(cidrs),
			newNodesSnapshot(defaultNodeAddresses),
		)

		values.SetMock.Set(func(path string, v any) {
			switch path {
			case settings.InternalValuesConfigCopyPath:
				Fail("Should not copy ModuleConfig settings")
			case settings.InternalValuesConfigValidationPath:
				Expect(v).To(HaveKey("error"))
			default:
				Fail("unexpected path")
			}
		})

		values.RemoveMock.Optional()

		Expect(Reconcile(context.Background(), newInput())).To(Succeed())

		Expect(values.RemoveMock.Calls()).To(HaveLen(0), "should not remove values")
	},
		Entry("contain node address", virtualMachineCIDRsOverlapWithNodeAddresses),
		Entry("overlap with podSubnet", virtualMachineCIDRsOverlapWithPodSubnet),
		Entry("overlap with serviceSubnet", virtualMachineCIDRsOverlapWithServiceSubnet),
	)
})
