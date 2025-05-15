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

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
)

func TestDiscoveryClusterIPServiceForDVCR(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DiscoveryClusterIPServiceForDVCR Suite")
}

var _ = Describe("DiscoveryClusterIPServiceForDVCR", func() {
	var (
		dc        *mock.DependencyContainerMock
		snapshots *mock.SnapshotsMock
		values    *mock.PatchableValuesCollectorMock
	)

	BeforeEach(func() {
		dc = mock.NewDependencyContainerMock(GinkgoT())
		snapshots = mock.NewSnapshotsMock(GinkgoT())
		values = mock.NewPatchableValuesCollectorMock(GinkgoT())
	})

	AfterEach(func() {
		dc = nil
		snapshots = nil
		values = nil
	})

	setSnapshots := func(snaps ...pkg.Snapshot) {
		snapshots.GetMock.When(discoveryService).Then(snaps)
	}

	newSnapshot := func(clusterIP string) pkg.Snapshot {
		return mock.NewSnapshotMock(GinkgoT()).StringMock.Set(func() (s1 string) {
			return clusterIP
		})
	}

	newInput := func() *pkg.HookInput {
		return &pkg.HookInput{
			Snapshots: snapshots,
			Values:    values,
			DC:        dc,
			Logger:    log.NewNop(),
		}
	}

	It("should set serviceIP to values", func() {
		setSnapshots(newSnapshot("10.0.0.1"))
		values.GetMock.When(serviceIPValuePath).Then(gjson.Result{Type: gjson.String, Str: ""})
		values.SetMock.Set(func(path string, v any) {
			Expect(path).To(Equal(serviceIPValuePath))
			Expect(v).To(Equal("10.0.0.1"))
		})
		Expect(handleDiscoveryService(context.Background(), newInput())).To(Succeed())
	})

	It("Should delete serviceIP from values", func() {
		setSnapshots(newSnapshot(""))
		values.RemoveMock.Set(func(path string) {
			Expect(path).To(Equal(serviceIPValuePath))
		})
		Expect(handleDiscoveryService(context.Background(), newInput())).To(Succeed())
	})
})
