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

package migrate_virthandler_kvm_node_labels

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
)

func createSnapshotMock(nodeInfo NodeInfo) pkg.Snapshot {
	m := mock.NewSnapshotMock(GinkgoT())
	m.UnmarshalToMock.Set(func(v any) error {
		target, ok := v.(*NodeInfo)
		if !ok {
			return fmt.Errorf("expected *NodeInfo, got %T", v)
		}
		*target = nodeInfo
		return nil
	})
	return m
}

func TestMigratevirtHandlerKVMLabels(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Migrate virtHandler KVM labels Suite")
}

var _ = Describe("Migrate virtHandler KVM labels", func() {
	var (
		snapshots      *mock.SnapshotsMock
		values         *mock.OutputPatchableValuesCollectorMock
		patchCollector *mock.PatchCollectorMock
		input          *pkg.HookInput
		buf            *bytes.Buffer
	)

	BeforeEach(func() {
		snapshots = mock.NewSnapshotsMock(GinkgoT())
		values = mock.NewPatchableValuesCollectorMock(GinkgoT())
		patchCollector = mock.NewPatchCollectorMock(GinkgoT())

		buf = bytes.NewBuffer([]byte{})

		input = &pkg.HookInput{
			Values:    values,
			Snapshots: snapshots,
			Logger: log.NewLogger(log.Options{
				Level:  log.LevelDebug.Level(),
				Output: buf,
				TimeFunc: func(_ time.Time) time.Time {
					parsedTime, err := time.Parse(time.DateTime, "2006-01-02 15:04:05")
					Expect(err).ShouldNot(HaveOccurred())
					return parsedTime
				},
			}),
			PatchCollector: patchCollector,
		}
	})

	Context("Empty cluster", func() {
		It("Hook must execute successfully", func() {
			snapshots.GetMock.When(nodesSnapshot).Then(
				[]pkg.Snapshot{},
			)
			err := handler(context.Background(), input)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	Context("Four nodes but only two should be patched.", func() {
		It("Hook must execute successfully", func() {
			expectedNodes := map[string]struct{}{
				"node1": {},
				"node4": {},
			}

			snapshots.GetMock.When(nodesSnapshot).Then([]pkg.Snapshot{
				// should be patched
				createSnapshotMock(NodeInfo{
					Name: "node1",
					Labels: map[string]string{
						"kubevirt.internal.virtualization.deckhouse.io/schedulable": "true",
					},
				}),
				// should not be patched
				createSnapshotMock(NodeInfo{
					Name: "node2",
					Labels: map[string]string{
						"kubevirt.internal.virtualization.deckhouse.io/schedulable": "true",
						"virtualization.deckhouse.io/kvm-enabled":                   "true",
					},
				}),
				// should not be patched
				createSnapshotMock(NodeInfo{
					Name: "node3",
					Labels: map[string]string{
						"kubevirt.internal.virtualization.deckhouse.io/schedulable": "true",
						"virtualization.deckhouse.io/kvm-enabled":                   "false",
					},
				}),
				// should be patched
				createSnapshotMock(NodeInfo{
					Name: "node4",
					Labels: map[string]string{
						"kubevirt.internal.virtualization.deckhouse.io/schedulable": "true",
					},
				}),
			})

			patchCollector.PatchWithJSONMock.Set(func(patch any, apiVersion, kind, namespace, name string, opts ...pkg.PatchCollectorOption) {
				p, ok := patch.([]map[string]string)
				Expect(ok).To(BeTrue())
				Expect(expectedNodes).To(HaveKey(name))
				Expect(p).To(BeEquivalentTo(kvmLabelPatch))
				delete(expectedNodes, name)
			})

			err := handler(context.Background(), input)
			Expect(err).ShouldNot(HaveOccurred())

			Expect(buf.String()).To(ContainSubstring(fmt.Sprintf(logMessageTemplate, kvmEnabledLabel, "node1")))
			Expect(buf.String()).To(ContainSubstring(fmt.Sprintf(logMessageTemplate, kvmEnabledLabel, "node4")))

			Expect(expectedNodes).To(HaveLen(0))
		})
	})
})
