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

package parallel_outbound_migrations_per_node

import (
	"context"
	"fmt"
	"maps"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
)

func TestParallelOutboundMigrationsPerNode(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ParallelOutboundMigrationsPerNode Suite")
}

var _ = Describe("ParallelOutboundMigrationsPerNode", func() {
	var (
		dc        *mock.DependencyContainerMock
		snapshots *mock.SnapshotsMock
		values    *mock.OutputPatchableValuesCollectorMock
	)

	setSnapshots := func(snaps ...pkg.Snapshot) {
		snapshots.GetMock.When(snapshotModuleConfig).Then(snaps)
	}

	newSnapshot := func(annos map[string]string) pkg.Snapshot {
		return mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) (err error) {
			data, ok := v.(*map[string]string)
			Expect(ok).To(BeTrue())
			*data = make(map[string]string)
			maps.Copy(*data, annos)
			return nil
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

	It("Should set parallel outbound migrations per node", func() {
		setSnapshots(newSnapshot(map[string]string{
			migrationsPerNodeAnnotationKey: "5",
		}))

		values.GetMock.When(migrationsPerNodeValuesPath).Then(gjson.Result{Type: gjson.Number, Num: 1})

		values.SetMock.Set(func(path string, v any) {
			value, ok := v.(int)
			Expect(ok).To(BeTrue())
			Expect(value).To(Equal(5))
		})

		Expect(reconcile(context.Background(), newInput())).To(Succeed())
	})

	It("Should set default parallel outbound migrations per node", func() {
		setSnapshots(newSnapshot(map[string]string{}))

		values.GetMock.When(migrationsPerNodeValuesPath).Then(gjson.Result{Type: gjson.Number, Num: 5})

		values.SetMock.Set(func(path string, v any) {
			value, ok := v.(int)
			Expect(ok).To(BeTrue())
			Expect(value).To(Equal(defaultMigrationsPerNode))
		})

		Expect(reconcile(context.Background(), newInput())).To(Succeed())
	})

	It("Should don't set parallel outbound migrations per node if it's already set", func() {
		setSnapshots(newSnapshot(map[string]string{
			migrationsPerNodeAnnotationKey: "5",
		}))

		values.GetMock.When(migrationsPerNodeValuesPath).Then(gjson.Result{Type: gjson.Number, Num: 5})
		Expect(reconcile(context.Background(), newInput())).To(Succeed())
	})

	It("Should finish with error because annotations is wrong", func() {
		setSnapshots(newSnapshot(map[string]string{
			migrationsPerNodeAnnotationKey: "wrong",
		}))

		Expect(reconcile(context.Background(), newInput())).To(MatchError(ContainSubstring(fmt.Sprintf("failed to parse %q annotation:", migrationsPerNodeAnnotationKey))))
	})
})
