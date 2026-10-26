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

package vdmigration

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

func TestVDMigration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VirtualDisk Migration Suite")
}

var _ = Describe("volume migration", func() {
	now := time.Now().Truncate(time.Second)

	disk := func(start, end time.Duration, result v1alpha2.VirtualDiskMigrationResult, reason string) *v1alpha2.VirtualDisk {
		vd := &v1alpha2.VirtualDisk{}
		vd.Status.MigrationState.StartTimestamp = metav1.NewTime(now.Add(start))
		if end != 0 {
			vd.Status.MigrationState.EndTimestamp = metav1.NewTime(now.Add(end))
			vd.Status.MigrationState.Result = result
		}
		if reason != "" {
			vd.Status.Conditions = []metav1.Condition{{Type: vdcondition.MigratingType.String(), Status: metav1.ConditionTrue, Reason: reason}}
		}
		return vd
	}

	DescribeTable("InSameMigration",
		func(other *v1alpha2.VirtualDisk, expected bool) {
			vd := disk(-time.Minute, 0, "", "")
			Expect(InSameMigration(vd, other, now)).To(Equal(expected))
		},
		Entry("migrating along", disk(-time.Minute, 0, "", ""), true),
		Entry("finished during the migration", disk(-time.Minute, -time.Second, v1alpha2.VirtualDiskMigrationResultSucceeded, ""), true),
		Entry("finished before the migration", disk(-10*time.Minute, -5*time.Minute, v1alpha2.VirtualDiskMigrationResultSucceeded, ""), false),
		Entry("failed before it began", disk(-time.Second, -time.Second, v1alpha2.VirtualDiskMigrationResultFailed, ""), false),
		Entry("never migrated", &v1alpha2.VirtualDisk{}, false),
	)

	DescribeTable("IsMigrationCompleting",
		func(vds map[string]*v1alpha2.VirtualDisk, expected bool) {
			Expect(IsMigrationCompleting(vds, now)).To(Equal(expected))
		},
		Entry("a disk waits for the source release", map[string]*v1alpha2.VirtualDisk{
			"a": disk(-time.Minute, 0, "", vdcondition.MigratingWaitForSourceVolumeReleaseReason.String()),
			"b": disk(-time.Minute, 0, "", vdcondition.MigratingInProgressReason.String()),
		}, true),
		Entry("a disk of the migration completed", map[string]*v1alpha2.VirtualDisk{
			"a": disk(-time.Minute, -time.Second, v1alpha2.VirtualDiskMigrationResultSucceeded, ""),
			"b": disk(-time.Minute, 0, "", vdcondition.MigratingInProgressReason.String()),
		}, true),
		Entry("only a disk of the previous migration completed", map[string]*v1alpha2.VirtualDisk{
			"a": disk(-10*time.Minute, -5*time.Minute, v1alpha2.VirtualDiskMigrationResultSucceeded, ""),
			"b": disk(-time.Minute, 0, "", vdcondition.MigratingInProgressReason.String()),
		}, false),
		Entry("the disks are still copying", map[string]*v1alpha2.VirtualDisk{
			"a": disk(-time.Minute, 0, "", vdcondition.MigratingInProgressReason.String()),
			"b": disk(-time.Minute, 0, "", vdcondition.MigratingInProgressReason.String()),
		}, false),
	)

	DescribeTable("IsMigrationReverting",
		func(reasonA string, deletedA bool, reasonB string, deletedB, expected bool) {
			vds := map[string]*v1alpha2.VirtualDisk{
				"a": disk(-time.Minute, 0, "", reasonA),
				"b": disk(-time.Minute, 0, "", reasonB),
			}
			if deletedA {
				vds["a"].DeletionTimestamp = ptr.To(metav1.Now())
			}
			if deletedB {
				vds["b"].DeletionTimestamp = ptr.To(metav1.Now())
			}
			Expect(IsMigrationReverting(vds)).To(Equal(expected))
		},
		Entry("every disk reverts",
			vdcondition.MigratingWaitForTargetVolumeReleaseReason.String(), false, vdcondition.MigratingWaitForTargetVolumeReleaseReason.String(), false, true),
		Entry("a disk is still migrating",
			vdcondition.MigratingWaitForTargetVolumeReleaseReason.String(), false, vdcondition.MigratingInProgressReason.String(), false, false),
		Entry("a disk completes",
			vdcondition.MigratingWaitForTargetVolumeReleaseReason.String(), false, vdcondition.MigratingWaitForSourceVolumeReleaseReason.String(), false, false),
		Entry("a disk still migrating is being deleted",
			vdcondition.MigratingWaitForTargetVolumeReleaseReason.String(), false, vdcondition.MigratingInProgressReason.String(), true, true),
	)

	DescribeTable("IsRevertedByOtherDisks",
		func(other *v1alpha2.VirtualDisk, otherDeleted, expected bool) {
			vd := disk(-time.Minute, 0, "", vdcondition.MigratingInProgressReason.String())
			vd.Name = "deleted"
			vd.DeletionTimestamp = ptr.To(metav1.Now())
			other.Name = "other"
			if otherDeleted {
				other.DeletionTimestamp = ptr.To(metav1.Now())
			}
			vds := map[string]*v1alpha2.VirtualDisk{vd.Name: vd, other.Name: other}
			Expect(IsRevertedByOtherDisks(vd, vds, now)).To(Equal(expected))
		},
		Entry("another disk reverts",
			disk(-time.Minute, 0, "", vdcondition.MigratingWaitForTargetVolumeReleaseReason.String()), false, true),
		Entry("another disk has reverted",
			disk(-time.Minute, -time.Second, v1alpha2.VirtualDiskMigrationResultFailed, ""), false, true),
		Entry("another disk is still migrating",
			disk(-time.Minute, 0, "", vdcondition.MigratingInProgressReason.String()), false, false),
		Entry("another disk completes",
			disk(-time.Minute, 0, "", vdcondition.MigratingWaitForSourceVolumeReleaseReason.String()), false, false),
		Entry("another disk has completed",
			disk(-time.Minute, -time.Second, v1alpha2.VirtualDiskMigrationResultSucceeded, ""), false, false),
		Entry("another disk reverted in a previous migration",
			disk(-10*time.Minute, -5*time.Minute, v1alpha2.VirtualDiskMigrationResultFailed, ""), false, false),
		Entry("the other disk reverts but is being deleted too",
			disk(-time.Minute, 0, "", vdcondition.MigratingWaitForTargetVolumeReleaseReason.String()), true, false),
	)
})
