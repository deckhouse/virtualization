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

		configValues         *mock.OutputPatchableValuesCollectorMock
		migrationNetworkType string
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
			Snapshots:    snapshots,
			Values:       values,
			ConfigValues: configValues,
			DC:           dc,
			Logger:       log.NewNop(),
		}
	}

	BeforeEach(func() {
		dc = mock.NewDependencyContainerMock(GinkgoT())
		snapshots = mock.NewSnapshotsMock(GinkgoT())
		values = mock.NewOutputPatchableValuesCollectorMock(GinkgoT())
		configValues = mock.NewOutputPatchableValuesCollectorMock(GinkgoT())
		migrationNetworkType = ""
		configValues.GetMock.Set(func(path string) gjson.Result {
			if path == liveMigrationNetworkTypeConfigPath && migrationNetworkType != "" {
				return gjson.Result{Type: gjson.String, Str: migrationNetworkType}
			}
			return gjson.Result{}
		})
	})

	AfterEach(func() {
		dc = nil
		snapshots = nil
		values = nil
		configValues = nil
	})

	It("Should set all migration params from annotations", func() {
		setSnapshots(newSnapshot(map[string]string{
			bandwidthPerMigrationAnnotation:              "1Gi",
			completionTimeoutPerGiBAnnotation:            "1200",
			maxActiveOutboundMigrationsPerNodeAnnotation: "4",
			progressTimeoutAnnotation:                    "300",
			disableTLSAnnotation:                         "true",
			disableFirmwareUpdateAnnotation:              "true",
		}))

		values.GetMock.Set(func(path string) gjson.Result {
			switch path {
			case bandwidthPerMigrationValuesPath:
				return gjson.Result{Type: gjson.String, Str: defaultBandwidthPerMigration}
			case completionTimeoutPerGiBValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultCompletionTimeoutPerGiB}
			case parallelOutboundMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultMaxActiveOutboundMigrationsPerNode + defaultMaxPreactiveOutboundMigrationsPerNode}
			case progressTimeoutValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultProgressTimeout}
			case disableTLSValuesPath:
				return gjson.Result{Type: gjson.False}
			case disableFirmwareUpdateValuesPath:
				return gjson.Result{Type: gjson.False}
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
		Expect(setValues).To(HaveKeyWithValue(maxActiveOutboundMigrationsPerNodeValuesPath, 4))
		Expect(setValues).To(HaveKeyWithValue(progressTimeoutValuesPath, 300))
		Expect(setValues).To(HaveKeyWithValue(disableTLSValuesPath, true))
		Expect(setValues).To(HaveKeyWithValue(disableFirmwareUpdateValuesPath, true))
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
			case disableTLSValuesPath:
				return gjson.Result{Type: gjson.True}
			case disableFirmwareUpdateValuesPath:
				return gjson.Result{Type: gjson.True}
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
		Expect(setValues).To(HaveKeyWithValue(parallelOutboundMigrationsPerNodeValuesPath, defaultMaxActiveOutboundMigrationsPerNode+defaultMaxPreactiveOutboundMigrationsPerNode))
		Expect(setValues).To(HaveKeyWithValue(maxActiveOutboundMigrationsPerNodeValuesPath, defaultMaxActiveOutboundMigrationsPerNode))
		Expect(setValues).To(HaveKeyWithValue(maxActiveInboundMigrationsPerNodeValuesPath, defaultMaxActiveInboundMigrationsPerNode))
		Expect(setValues).To(HaveKeyWithValue(progressTimeoutValuesPath, defaultProgressTimeout))
		Expect(setValues).To(HaveKeyWithValue(disableTLSValuesPath, defaultDisableTLS))
		Expect(setValues).To(HaveKeyWithValue(disableFirmwareUpdateValuesPath, defaultDisableFirmwareUpdate))
	})

	It("Should not set values when current matches target", func() {
		setSnapshots(newSnapshot(map[string]string{
			bandwidthPerMigrationAnnotation:                 defaultBandwidthPerMigration,
			completionTimeoutPerGiBAnnotation:               "800",
			maxActiveOutboundMigrationsPerNodeAnnotation:    "1",
			maxActiveInboundMigrationsPerNodeAnnotation:     "1",
			maxPreactiveOutboundMigrationsPerNodeAnnotation: "1",
			progressTimeoutAnnotation:                       "150",
			disableTLSAnnotation:                            "false",
			disableFirmwareUpdateAnnotation:                 "false",
		}))

		values.GetMock.Set(func(path string) gjson.Result {
			switch path {
			case bandwidthPerMigrationValuesPath:
				return gjson.Result{Type: gjson.String, Str: defaultBandwidthPerMigration}
			case completionTimeoutPerGiBValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultCompletionTimeoutPerGiB}
			case parallelOutboundMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultMaxActiveOutboundMigrationsPerNode + defaultMaxPreactiveOutboundMigrationsPerNode}
			case maxActiveOutboundMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultMaxActiveOutboundMigrationsPerNode}
			case maxActiveInboundMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultMaxActiveInboundMigrationsPerNode}
			case maxActiveMigrationsPerClusterValuesPath:
				return gjson.Result{Type: gjson.String, Str: defaultMaxActiveMigrationsPerCluster}
			case maxActiveMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: 0}
			case progressTimeoutValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultProgressTimeout}
			case disableTLSValuesPath:
				return gjson.Result{Type: gjson.False}
			case disableFirmwareUpdateValuesPath:
				return gjson.Result{Type: gjson.False}
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
			case disableTLSValuesPath:
				return gjson.Result{Type: gjson.False}
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

	It("Should fail on invalid boolean annotation", func() {
		setSnapshots(newSnapshot(map[string]string{
			disableTLSAnnotation: "not-a-bool",
		}))

		values.GetMock.Set(func(path string) gjson.Result {
			switch path {
			case bandwidthPerMigrationValuesPath:
				return gjson.Result{Type: gjson.String, Str: defaultBandwidthPerMigration}
			case completionTimeoutPerGiBValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultCompletionTimeoutPerGiB}
			case parallelOutboundMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultMaxActiveOutboundMigrationsPerNode + defaultMaxPreactiveOutboundMigrationsPerNode}
			case maxActiveOutboundMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultMaxActiveOutboundMigrationsPerNode}
			case maxActiveInboundMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultMaxActiveInboundMigrationsPerNode}
			case maxActiveMigrationsPerClusterValuesPath:
				return gjson.Result{Type: gjson.String, Str: defaultMaxActiveMigrationsPerCluster}
			case progressTimeoutValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultProgressTimeout}
			case disableTLSValuesPath:
				return gjson.Result{Type: gjson.False}
			default:
				return gjson.Result{}
			}
		})

		err := reconcile(context.Background(), newInput())
		Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf(
			"failed to parse %q annotation:",
			disableTLSAnnotation,
		))))
	})

	It("Should set only one param from annotation and defaults for the rest", func() {
		setSnapshots(newSnapshot(map[string]string{
			maxPreactiveOutboundMigrationsPerNodeAnnotation: "4",
		}))

		values.GetMock.Set(func(path string) gjson.Result {
			switch path {
			case bandwidthPerMigrationValuesPath:
				return gjson.Result{Type: gjson.String, Str: defaultBandwidthPerMigration}
			case completionTimeoutPerGiBValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultCompletionTimeoutPerGiB}
			case parallelOutboundMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultMaxActiveOutboundMigrationsPerNode + defaultMaxPreactiveOutboundMigrationsPerNode}
			case maxActiveOutboundMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultMaxActiveOutboundMigrationsPerNode}
			case maxActiveInboundMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultMaxActiveInboundMigrationsPerNode}
			case maxActiveMigrationsPerClusterValuesPath:
				return gjson.Result{Type: gjson.String, Str: defaultMaxActiveMigrationsPerCluster}
			case maxActiveMigrationsPerNodeValuesPath:
				return gjson.Result{Type: gjson.Number, Num: 0}
			case progressTimeoutValuesPath:
				return gjson.Result{Type: gjson.Number, Num: defaultProgressTimeout}
			case disableTLSValuesPath:
				return gjson.Result{Type: gjson.False}
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

	Describe("migration slots", func() {
		reconcileSlots := func(annos map[string]string) (map[string]any, error) {
			setSnapshots(newSnapshot(annos))
			values.GetMock.Set(func(string) gjson.Result { return gjson.Result{} })
			setValues := map[string]any{}
			values.SetMock.Set(func(path string, v any) {
				setValues[path] = v
			})
			err := reconcile(context.Background(), newInput())
			return setValues, err
		}

		It("derives the outbound window from the active and preactive outgoing migrations", func() {
			setValues, err := reconcileSlots(map[string]string{
				maxActiveOutboundMigrationsPerNodeAnnotation:    "2",
				maxActiveInboundMigrationsPerNodeAnnotation:     "3",
				maxPreactiveOutboundMigrationsPerNodeAnnotation: "2",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(setValues).To(HaveKeyWithValue(parallelOutboundMigrationsPerNodeValuesPath, 4))
			Expect(setValues).To(HaveKeyWithValue(maxActiveOutboundMigrationsPerNodeValuesPath, 2))
			Expect(setValues).To(HaveKeyWithValue(maxActiveInboundMigrationsPerNodeValuesPath, 3))
		})

		It("allows turning preactive migrations off", func() {
			setValues, err := reconcileSlots(map[string]string{
				maxPreactiveOutboundMigrationsPerNodeAnnotation: "0",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(setValues).To(HaveKeyWithValue(parallelOutboundMigrationsPerNodeValuesPath, defaultMaxActiveOutboundMigrationsPerNode))
		})

		It("sizes the outbound window from the shared budget when it is set", func() {
			setValues, err := reconcileSlots(map[string]string{
				maxActiveMigrationsPerNodeAnnotation:            "3",
				maxPreactiveOutboundMigrationsPerNodeAnnotation: "1",
				maxActiveOutboundMigrationsPerNodeAnnotation:    "5",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(setValues).To(HaveKeyWithValue(maxActiveMigrationsPerNodeValuesPath, 3))
			Expect(setValues).To(HaveKeyWithValue(parallelOutboundMigrationsPerNodeValuesPath, 4))
		})

		It("ignores the removed parallel-* annotations", func() {
			setValues, err := reconcileSlots(map[string]string{
				"virtualization.deckhouse.io/parallel-outbound-migrations-per-node": "12",
				"virtualization.deckhouse.io/parallel-sync-migrations-per-node":     "3",
				"virtualization.deckhouse.io/parallel-inbound-migrations-per-node":  "12",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(setValues).To(HaveKeyWithValue(parallelOutboundMigrationsPerNodeValuesPath, defaultMaxActiveOutboundMigrationsPerNode+defaultMaxPreactiveOutboundMigrationsPerNode))
			Expect(setValues).To(HaveKeyWithValue(maxActiveOutboundMigrationsPerNodeValuesPath, defaultMaxActiveOutboundMigrationsPerNode))
			Expect(setValues).To(HaveKeyWithValue(maxActiveInboundMigrationsPerNodeValuesPath, defaultMaxActiveInboundMigrationsPerNode))
		})

		It("keeps the shared budget off by default", func() {
			setValues, err := reconcileSlots(map[string]string{})

			Expect(err).NotTo(HaveOccurred())
			Expect(setValues).To(HaveKeyWithValue(maxActiveMigrationsPerNodeValuesPath, 0))
		})

		It("turns the per-node limits off with disabled", func() {
			setValues, err := reconcileSlots(map[string]string{
				maxActiveOutboundMigrationsPerNodeAnnotation: "disabled",
				maxActiveInboundMigrationsPerNodeAnnotation:  "disabled",
				maxActiveMigrationsPerNodeAnnotation:         "disabled",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(setValues).To(HaveKeyWithValue(maxActiveOutboundMigrationsPerNodeValuesPath, 0))
			Expect(setValues).To(HaveKeyWithValue(maxActiveInboundMigrationsPerNodeValuesPath, 0))
			Expect(setValues).To(HaveKeyWithValue(maxActiveMigrationsPerNodeValuesPath, 0))
			Expect(setValues).To(HaveKeyWithValue(parallelOutboundMigrationsPerNodeValuesPath, 0), "the outbound window is not limited either")
		})

		It("keeps the shared budget as the outbound window when the separate outbound limit is disabled", func() {
			setValues, err := reconcileSlots(map[string]string{
				maxActiveOutboundMigrationsPerNodeAnnotation: "disabled",
				maxActiveMigrationsPerNodeAnnotation:         "2",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(setValues).To(HaveKeyWithValue(parallelOutboundMigrationsPerNodeValuesPath, 2+defaultMaxPreactiveOutboundMigrationsPerNode))
		})

		It("ignores the removed disable switches", func() {
			setValues, err := reconcileSlots(map[string]string{
				"virtualization.deckhouse.io/inbound-migration-limit":           "disabled",
				"virtualization.deckhouse.io/outbound-migration-limit":          "disabled",
				"virtualization.deckhouse.io/parallel-per-node-migration-limit": "disabled",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(setValues).To(HaveKeyWithValue(maxActiveOutboundMigrationsPerNodeValuesPath, defaultMaxActiveOutboundMigrationsPerNode))
			Expect(setValues).To(HaveKeyWithValue(maxActiveInboundMigrationsPerNodeValuesPath, defaultMaxActiveInboundMigrationsPerNode))
		})

		DescribeTable("rejects values below the minimum",
			func(annotation, value string) {
				_, err := reconcileSlots(map[string]string{annotation: value})
				Expect(err).To(MatchError(ContainSubstring(annotation)))
			},
			Entry("active outbound", maxActiveOutboundMigrationsPerNodeAnnotation, "0"),
			Entry("active inbound", maxActiveInboundMigrationsPerNodeAnnotation, "0"),
			Entry("preactive outbound", maxPreactiveOutboundMigrationsPerNodeAnnotation, "-1"),
			Entry("shared budget", maxActiveMigrationsPerNodeAnnotation, "0"),
			Entry("cluster limit", maxActiveMigrationsPerClusterAnnotation, "0"),
			Entry("cluster limit that is not a number", maxActiveMigrationsPerClusterAnnotation, "many"),
			Entry("active outbound that is not a number", maxActiveOutboundMigrationsPerNodeAnnotation, "off"),
		)
	})

	It("Should set the per-cluster migration limit and the inbound migration scheduler to disabled", func() {
		setSnapshots(newSnapshot(map[string]string{
			maxActiveMigrationsPerClusterAnnotation: "disabled",
			inboundMigrationSchedulerAnnotation:     "disabled",
		}))

		values.GetMock.Set(func(path string) gjson.Result {
			switch path {
			case maxActiveMigrationsPerClusterValuesPath, inboundMigrationSchedulerValuesPath:
				return gjson.Result{Type: gjson.String, Str: ""}
			default:
				return gjson.Result{}
			}
		})

		setValues := map[string]any{}
		values.SetMock.Set(func(path string, v any) {
			setValues[path] = v
		})

		Expect(reconcile(context.Background(), newInput())).To(Succeed())

		Expect(setValues).To(HaveKeyWithValue(maxActiveMigrationsPerClusterValuesPath, "disabled"))
		Expect(setValues).To(HaveKeyWithValue(inboundMigrationSchedulerValuesPath, "disabled"))
	})

	It("Should not limit bandwidth by default when a dedicated migration network is configured", func() {
		migrationNetworkType = "SystemNetwork"
		setSnapshots(newSnapshot(map[string]string{}))

		values.GetMock.Set(func(path string) gjson.Result {
			if path == bandwidthPerMigrationValuesPath {
				return gjson.Result{Type: gjson.String, Str: defaultBandwidthPerMigration}
			}
			return gjson.Result{}
		})

		setValues := map[string]any{}
		values.SetMock.Set(func(path string, v any) {
			setValues[path] = v
		})

		Expect(reconcile(context.Background(), newInput())).To(Succeed())

		Expect(setValues).To(HaveKeyWithValue(bandwidthPerMigrationValuesPath, unlimitedBandwidthPerMigration))
		Expect(setValues).To(HaveKeyWithValue(parallelOutboundMigrationsPerNodeValuesPath, defaultMaxActiveOutboundMigrationsPerNode+defaultMaxPreactiveOutboundMigrationsPerNode))
	})

	It("Should prefer the bandwidth annotation over the dedicated migration network default", func() {
		migrationNetworkType = "SystemNetwork"
		setSnapshots(newSnapshot(map[string]string{
			bandwidthPerMigrationAnnotation: "1Gi",
		}))

		values.GetMock.Set(func(path string) gjson.Result {
			return gjson.Result{}
		})

		setValues := map[string]any{}
		values.SetMock.Set(func(path string, v any) {
			setValues[path] = v
		})

		Expect(reconcile(context.Background(), newInput())).To(Succeed())

		Expect(setValues).To(HaveKeyWithValue(bandwidthPerMigrationValuesPath, "1Gi"))
	})
})
