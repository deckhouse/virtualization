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
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

func TestCleanUpVirtHandlerNodeLabels(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cleanup virtHandler node labels Suite")
}

var _ = Describe("Cleanup virtHandler node labels", func() {
	var (
		snapshots      *mock.SnapshotsMock
		values         *mock.PatchableValuesCollectorMock
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
			err := handleCleanUpNodeLabels(context.Background(), input)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	Context("Nodes should be patched.", func() {
		It("Hook must execute successfully", func() {
			expectedNodes := map[string]interface{}{
				"node1": []map[string]string{
					map[string]string{
						"op":   "remove",
						"path": "/metadata/labels/kubevirt.internal.virtualization.deckhouse.io~1schedulable",
					},
				},
				"node2": []map[string]string{
					{
						"op":   "remove",
						"path": "/metadata/labels/kubevirt.internal.virtualization.deckhouse.io~1schedulable",
					},
					map[string]string{
						"op":   "remove",
						"path": "/metadata/labels/virtualization.deckhouse.io~1kvm-enabled",
					},
				},
			}

			snapshots.GetMock.When(nodesSnapshot).Then([]pkg.Snapshot{
				// should be patched
				createSnapshotMock(NodeInfo{
					Name: "node1",
					Labels: map[string]string{
						"kubevirt.internal.virtualization.deckhouse.io/schedulable": "true",
					},
				}),
				// should be patched
				createSnapshotMock(NodeInfo{
					Name: "node2",
					Labels: map[string]string{
						"kubevirt.internal.virtualization.deckhouse.io/schedulable": "true",
						"virtualization.deckhouse.io/kvm-enabled":                   "true",
					},
				}),
			})

			patchCollector.PatchWithJSONMock.Set(func(patch any, apiVersion, kind, namespace, name string, opts ...pkg.PatchCollectorOption) {
				p, ok := patch.([]map[string]string)
				Expect(ok).To(BeTrue())
				Expect(expectedNodes).To(HaveKey(name))
				Expect(p).To(Equal(expectedNodes[name]))
				delete(expectedNodes, name)
			})

			err := handleCleanUpNodeLabels(context.Background(), input)
			Expect(err).ShouldNot(HaveOccurred())

			Expect(buf.String()).To(ContainSubstring(fmt.Sprintf(logMessageTemplate, 1, labelPattern, "node1")))
			Expect(buf.String()).To(ContainSubstring(fmt.Sprintf(logMessageTemplate, 2, labelPattern, "node2")))

			Expect(expectedNodes).To(HaveLen(0))
		})
	})
})
