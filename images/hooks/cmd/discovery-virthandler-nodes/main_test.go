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
	"os"
	"testing"
	"time"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDiscoveryVirthandlerNodes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Discovery virt-handler nodes Suite")
}

var _ = Describe("Discovery virt-handler nodes", func() {
	err := os.Setenv("D8_IS_TESTS_ENVIRONMENT", "true")
	Expect(err).ShouldNot(HaveOccurred())

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
			values.SetMock.When(virtHandlerNodeCountPath, 0)
			err := handleDiscoveryVirtHandkerNodes(context.Background(), input)
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
			err := handleDiscoveryVirtHandkerNodes(context.Background(), input)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})

})
