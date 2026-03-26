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

package migration_config

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

func TestMigrationConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MigrationConfig Suite")
}

var _ = Describe("MigrationConfig", func() {
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

	It("Should set all migration params from annotations", func() {
		setSnapshots(newSnapshot(map[string]string{
			bandwidthPerMigrationAnnotation:             "1Gi",
			completionTimeoutPerGiBAnnotation:           "1200",
			parallelOutboundMigrationsPerNodeAnnotation: "5",
			progressTimeoutAnnotation:                   "300",
		}))

		values.GetMock.Set(func(path string) gjson.Result {
			switch path {
			case bandwidthPerMigrationValuesPath:
				return gjson.Result{Type: gjson.String, Str: defaultBandwidthPerMigration}
			case completionTimeoutPerGiBValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultCompletionTimeoutPerGiB}
			case parallelOutboundMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultParallelOutboundMigrationsPerNode}
			case progressTimeoutValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultProgressTimeout}
			}
			return gjson.Result{}
		})

		setValues := map[string]any{}
		values.SetMock.Set(func(path string, v any) {
			setValues[path] = v
		})

		Expect(reconcile(context.Background(), newInput())).To(Succeed())

		Expect(setValues).To(HaveKeyWithValue(bandwidthPerMigrationValuesPath, "1Gi"))
		Expect(setValues).To(HaveKeyWithValue(completionTimeoutPerGiBValuesPath, 1200))
		Expect(setValues).To(HaveKeyWithValue(parallelOutboundMigrationsPerNodeValuesPath, 5))
		Expect(setValues).To(HaveKeyWithValue(progressTimeoutValuesPath, 300))
	})

	It("Should set defaults when no annotations present", func() {
		setSnapshots(newSnapshot(map[string]string{}))

		values.GetMock.Set(func(path string) gjson.Result {
			switch path {
			case bandwidthPerMigrationValuesPath:
				return gjson.Result{Type: gjson.String, Str: "1Gi"}
			case completionTimeoutPerGiBValuesPath:
				return gjson.Result{Type: gjson.Number, Num: 9999}
			case parallelOutboundMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: 9999}
			case progressTimeoutValuesPath:
				return gjson.Result{Type: gjson.Number, Num: 9999}
			}
			return gjson.Result{}
		})

		setValues := map[string]any{}
		values.SetMock.Set(func(path string, v any) {
			setValues[path] = v
		})

		Expect(reconcile(context.Background(), newInput())).To(Succeed())

		Expect(setValues).To(HaveKeyWithValue(bandwidthPerMigrationValuesPath, defaultBandwidthPerMigration))
		Expect(setValues).To(HaveKeyWithValue(completionTimeoutPerGiBValuesPath, defaultCompletionTimeoutPerGiB))
		Expect(setValues).To(HaveKeyWithValue(parallelOutboundMigrationsPerNodeValuesPath, defaultParallelOutboundMigrationsPerNode))
		Expect(setValues).To(HaveKeyWithValue(progressTimeoutValuesPath, defaultProgressTimeout))
	})

	It("Should not set values when current matches target", func() {
		setSnapshots(newSnapshot(map[string]string{
			bandwidthPerMigrationAnnotation:             defaultBandwidthPerMigration,
			completionTimeoutPerGiBAnnotation:           "800",
			parallelOutboundMigrationsPerNodeAnnotation: "1",
			progressTimeoutAnnotation:                   "150",
		}))

		values.GetMock.Set(func(path string) gjson.Result {
			switch path {
			case bandwidthPerMigrationValuesPath:
				return gjson.Result{Type: gjson.String, Str: defaultBandwidthPerMigration}
			case completionTimeoutPerGiBValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultCompletionTimeoutPerGiB}
			case parallelOutboundMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultParallelOutboundMigrationsPerNode}
			case progressTimeoutValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultProgressTimeout}
			}
			return gjson.Result{}
		})

		Expect(reconcile(context.Background(), newInput())).To(Succeed())
	})

	It("Should fail on invalid integer annotation", func() {
		setSnapshots(newSnapshot(map[string]string{
			completionTimeoutPerGiBAnnotation: "invalid",
		}))

		values.GetMock.Set(func(path string) gjson.Result {
			switch path {
			case bandwidthPerMigrationValuesPath:
				return gjson.Result{Type: gjson.String, Str: defaultBandwidthPerMigration}
			default:
				return gjson.Result{Type: gjson.Number, Num: defaultCompletionTimeoutPerGiB}
			}
		})

		err := reconcile(context.Background(), newInput())
		Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf(
			"failed to parse %q annotation:",
			completionTimeoutPerGiBAnnotation,
		))))
	})

	It("Should set only one param from annotation and defaults for the rest", func() {
		setSnapshots(newSnapshot(map[string]string{
			parallelOutboundMigrationsPerNodeAnnotation: "5",
		}))

		values.GetMock.Set(func(path string) gjson.Result {
			switch path {
			case bandwidthPerMigrationValuesPath:
				return gjson.Result{Type: gjson.String, Str: defaultBandwidthPerMigration}
			case completionTimeoutPerGiBValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultCompletionTimeoutPerGiB}
			case parallelOutboundMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultParallelOutboundMigrationsPerNode}
			case progressTimeoutValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultProgressTimeout}
			}
			return gjson.Result{}
		})

		setValues := map[string]any{}
		values.SetMock.Set(func(path string, v any) {
			setValues[path] = v
		})

		Expect(reconcile(context.Background(), newInput())).To(Succeed())

		Expect(setValues).To(HaveLen(1))
		Expect(setValues).To(HaveKeyWithValue(parallelOutboundMigrationsPerNodeValuesPath, 5))
	})
})
