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

// Package vdmigration keeps a volume migration of several disks all-or-nothing: every disk ends up
// either completed or reverted.
package vdmigration

import (
	"slices"
	"time"

	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

func IsMigrating(vd *v1alpha2.VirtualDisk) bool {
	return vd != nil && !vd.Status.MigrationState.StartTimestamp.IsZero() && vd.Status.MigrationState.EndTimestamp.IsZero()
}

// A disk waiting for its source release is never reverted.
func IsWaitingForSourceRelease(vd *v1alpha2.VirtualDisk) bool {
	return IsMigrating(vd) && migratingReason(vd) == vdcondition.MigratingWaitForSourceVolumeReleaseReason.String()
}

func IsWaitingForTargetRelease(vd *v1alpha2.VirtualDisk) bool {
	return IsMigrating(vd) && migratingReason(vd) == vdcondition.MigratingWaitForTargetVolumeReleaseReason.String()
}

func IsSucceeded(vd *v1alpha2.VirtualDisk) bool {
	return !IsMigrating(vd) && vd.Status.MigrationState.Result == v1alpha2.VirtualDiskMigrationResultSucceeded
}

func IsSwitchedToTarget(vd *v1alpha2.VirtualDisk) bool {
	return IsWaitingForSourceRelease(vd) || IsSucceeded(vd)
}

func IsTargetInKVVM(vd *v1alpha2.VirtualDisk, kvvm *virtv1.VirtualMachine) bool {
	if vd == nil || vd.Status.MigrationState.TargetPVC == "" || kvvm == nil || kvvm.Spec.Template == nil {
		return false
	}
	return slices.ContainsFunc(kvvm.Spec.Template.Spec.Volumes, func(v virtv1.Volume) bool {
		return v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == vd.Status.MigrationState.TargetPVC
	})
}

// Once KubeVirt has the target, a detached disk would finalize apart from the other disks of its
// migration. A disk being deleted never finalizes its migration, so waiting for it would hold it forever.
func ShouldDelayDetach(vd *v1alpha2.VirtualDisk, kvvm *virtv1.VirtualMachine) bool {
	return IsMigrating(vd) && vd.GetDeletionTimestamp().IsZero() && IsTargetInKVVM(vd, kvvm)
}

// A volume migration has no identifier: migrations of one VirtualMachine never overlap, so overlapping
// intervals mean the same one. Start equal to end is a migration that failed before it began.
func InSameMigration(vd, other *v1alpha2.VirtualDisk, now time.Time) bool {
	vdStart, otherStart := vd.Status.MigrationState.StartTimestamp, other.Status.MigrationState.StartTimestamp
	if vdStart.IsZero() || otherStart.IsZero() {
		return false
	}

	vdEnd, otherEnd := migrationEnd(vd, now), migrationEnd(other, now)
	if !otherEnd.After(otherStart.Time) {
		return false
	}

	return otherStart.Time.Before(vdEnd) && vdStart.Time.Before(otherEnd)
}

// A disk being deleted decides nothing, so it is not waited for.
func IsMigrationReverting(vds map[string]*v1alpha2.VirtualDisk) bool {
	for _, vd := range vds {
		if IsMigrating(vd) && vd.GetDeletionTimestamp().IsZero() && !IsWaitingForTargetRelease(vd) {
			return false
		}
	}
	return true
}

// The finished disks count too: once the others have reverted, the target of a disk being deleted
// would start the migration again.
func IsRevertedByOtherDisks(vd *v1alpha2.VirtualDisk, vds map[string]*v1alpha2.VirtualDisk, now time.Time) bool {
	var reverted bool
	for _, other := range vds {
		if other == nil || other.Name == vd.Name || !other.GetDeletionTimestamp().IsZero() || !InSameMigration(vd, other, now) {
			continue
		}
		switch {
		case IsWaitingForTargetRelease(other), isFailed(other):
			reverted = true
		case IsMigrating(other), IsSucceeded(other):
			return false
		}
	}
	return reverted
}

func IsMigrationCompleting(vds map[string]*v1alpha2.VirtualDisk, now time.Time) bool {
	for _, vd := range vds {
		if !IsMigrating(vd) {
			continue
		}
		for _, other := range vds {
			if IsSwitchedToTarget(other) && InSameMigration(vd, other, now) {
				return true
			}
		}
	}
	return false
}

func isFailed(vd *v1alpha2.VirtualDisk) bool {
	return !IsMigrating(vd) && vd.Status.MigrationState.Result == v1alpha2.VirtualDiskMigrationResultFailed
}

func migratingReason(vd *v1alpha2.VirtualDisk) string {
	cond, _ := conditions.GetCondition(vdcondition.MigratingType, vd.Status.Conditions)
	return cond.Reason
}

func migrationEnd(vd *v1alpha2.VirtualDisk, now time.Time) time.Time {
	if vd.Status.MigrationState.EndTimestamp.IsZero() {
		return now
	}
	return vd.Status.MigrationState.EndTimestamp.Time
}
