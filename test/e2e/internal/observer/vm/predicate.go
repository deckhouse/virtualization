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

package vm

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

// BeRunning reports the VirtualMachine has reached the Running phase. Intended
// for use with [Observer.WaitFor].
func BeRunning() Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		return vm.Status.Phase == v1alpha2.MachineRunning, nil
	}
}

// BeStopped reports the VirtualMachine has reached the Stopped phase. Intended
// for use with [Observer.WaitFor].
func BeStopped() Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		return vm.Status.Phase == v1alpha2.MachineStopped, nil
	}
}

// BeAgentReady reports the VirtualMachine's guest agent is ready, i.e. the
// AgentReady condition is present with Status=True. Intended for use with
// [Observer.WaitFor].
func BeAgentReady() Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond := findCondition(vm.Status.Conditions, vmcondition.TypeAgentReady.String())
		if cond == nil {
			return false, nil
		}
		return cond.Status == metav1.ConditionTrue, nil
	}
}

// HaveMigrationSucceeded reports the VirtualMachine's live migration has
// finished successfully: the migration state has an end timestamp and the
// Succeeded result. A Failed result is reported as a definite error (with the
// Migrating condition's reason and message) so a WaitFor caller fails
// immediately instead of waiting out the timeout. Intended for use with
// [Observer.WaitFor].
func HaveMigrationSucceeded() Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		state := vm.Status.MigrationState
		if state == nil || state.EndTimestamp.IsZero() {
			return false, nil
		}
		switch state.Result {
		case v1alpha2.MigrationResultSucceeded:
			return true, nil
		case v1alpha2.MigrationResultFailed:
			reason, message := "", ""
			if migrating := findCondition(vm.Status.Conditions, vmcondition.TypeMigrating.String()); migrating != nil {
				reason, message = migrating.Reason, migrating.Message
			}
			return false, fmt.Errorf("migration failed: reason: %s, message: %s", reason, message)
		default:
			return false, nil
		}
	}
}

// BeFilesystemFrozen reports the VirtualMachine's FilesystemFrozen condition is
// present with Status=True (the guest filesystem is frozen for a consistent
// snapshot). Freezing is transient, so observe it with [Observer.WaitFor]
// started before the snapshot is created.
func BeFilesystemFrozen() Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond := findCondition(vm.Status.Conditions, vmcondition.TypeFilesystemFrozen.String())
		if cond == nil {
			return false, nil
		}
		return cond.Status == metav1.ConditionTrue, nil
	}
}

// BeRebootedAfter reports the VirtualMachine is Running again with the
// Running condition transitioned after previousRunningTime, i.e. the guest
// went down and came back. Intended for use with [Observer.WaitFor].
func BeRebootedAfter(previousRunningTime time.Time) Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond := findCondition(vm.Status.Conditions, vmcondition.TypeRunning.String())
		if cond == nil {
			return false, nil
		}
		return cond.LastTransitionTime.After(previousRunningTime) && vm.Status.Phase == v1alpha2.MachineRunning, nil
	}
}

// BeAwaitingRestart reports the VirtualMachine is parked awaiting a manual
// restart to apply pending disruptive changes: the
// AwaitingRestartToApplyConfiguration condition is True and the pending
// changes are recorded in the status. Intended for use with
// [Observer.WaitFor].
func BeAwaitingRestart() Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond := findCondition(vm.Status.Conditions, vmcondition.TypeAwaitingRestartToApplyConfiguration.String())
		if cond == nil || cond.Status != metav1.ConditionTrue {
			return false, nil
		}
		return vm.Status.RestartAwaitingChanges != nil, nil
	}
}

// HaveBlockDevicesAttached reports every named VirtualDisk is attached to the
// VirtualMachine. Intended for use with [Observer.WaitFor].
func HaveBlockDevicesAttached(names ...string) Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		attached := make(map[string]bool, len(vm.Status.BlockDeviceRefs))
		for _, bd := range vm.Status.BlockDeviceRefs {
			if bd.Kind == v1alpha2.DiskDevice && bd.Attached {
				attached[bd.Name] = true
			}
		}
		for _, name := range names {
			if !attached[name] {
				return false, nil
			}
		}
		return true, nil
	}
}

// HaveBlockDeviceDetached reports the named VirtualDisk is no longer attached
// to the VirtualMachine. Intended for use with [Observer.WaitFor].
func HaveBlockDeviceDetached(name string) Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		for _, bd := range vm.Status.BlockDeviceRefs {
			if bd.Kind == v1alpha2.DiskDevice && bd.Name == name && bd.Attached {
				return false, nil
			}
		}
		return true, nil
	}
}

// HaveAttachedBlockDeviceCount reports the VirtualMachine status lists exactly
// count attached block devices (of any kind). Intended for use with
// [Observer.WaitFor].
func HaveAttachedBlockDeviceCount(count int) Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		attached := 0
		for _, bd := range vm.Status.BlockDeviceRefs {
			if bd.Attached {
				attached++
			}
		}
		return attached == count, nil
	}
}

// HaveActivePod reports the VirtualMachine status lists an active
// virt-launcher pod. Intended for use with [Observer.WaitFor].
func HaveActivePod() Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		for _, pod := range vm.Status.VirtualMachinePods {
			if pod.Active {
				return true, nil
			}
		}
		return false, nil
	}
}

// HaveMigrationStarted reports the VirtualMachine's migration state carries a
// start timestamp, i.e. a live migration has begun (and may have already
// finished). Intended for use with [Observer.WaitFor].
func HaveMigrationStarted() Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		state := vm.Status.MigrationState
		return state != nil && !state.StartTimestamp.IsZero(), nil
	}
}

// HaveMigrationInProgress reports a live migration is currently running: the
// migration state exists and has no end timestamp yet.
//
// The start timestamp is deliberately not consulted: KubeVirt does not
// reliably stamp it while the migration runs (the source virt-handler can
// miss it), and the fallback that backfills it does so at completion,
// together with the end timestamp - so "started but not ended" may never be
// observable at all. The migration state itself appears only once a migration
// is actually being processed, and after it finishes it lingers with the end
// timestamp set, so its bare presence without an end timestamp is the
// reliable in-flight signal.
//
// Migration is transient, so observe it with [Observer.WaitFor] started
// before the migration is triggered.
func HaveMigrationInProgress() Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		state := vm.Status.MigrationState
		return state != nil && state.EndTimestamp.IsZero(), nil
	}
}

// BeFailed reports an invariant violation when the VirtualMachine has entered
// the terminal Degraded phase or its Running condition reports
// InternalVirtualMachineError. Intended for use with [Observer.Never].
//
// The phase alone is not enough: the controller collapses every failing
// KubeVirt status (ErrImagePull, PvcNotFound, DataVolumeError,
// CrashLoopBackOff, ...) into the Pending phase, which is indistinguishable
// from a machine that is merely still starting. Only the Running condition
// keeps the distinction, so it is the condition — not the phase — that lets a
// spec fail at once instead of blocking until its wait times out.
//
// Running=False/PodNotStarted is deliberately NOT treated as a failure: it also
// covers a launcher pod that is momentarily Unschedulable, which a loaded
// cluster resolves on its own once room frees up.
func BeFailed() Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		if vm.Status.Phase == v1alpha2.MachineDegraded {
			return true, fmt.Errorf("VirtualMachine entered Degraded phase")
		}
		cond := findCondition(vm.Status.Conditions, vmcondition.TypeRunning.String())
		if cond != nil &&
			cond.Reason == vmcondition.ReasonInternalVirtualMachineError.String() &&
			cond.Message != temporarilyUnknownStateMessage {
			return true, fmt.Errorf("VirtualMachine reports an internal error: %s", cond.Message)
		}
		return false, nil
	}
}

// temporarilyUnknownStateMessage is the Running condition message the
// controller reports for an internal state it cannot determine (for example
// while the node hosting the machine is briefly unreachable). It shares the
// InternalVirtualMachineError reason with the genuinely terminal failures, and
// the message is the only thing that tells them apart, so [BeFailed] matches on
// it to avoid failing a spec over a blip that resolves on its own.
const temporarilyUnknownStateMessage = "The virtual machine state is temporarily unknown."

// HaveNoBootableDevice reports an invariant violation when the VirtualMachine's
// Running condition reports NoBootableDevice: the firmware scanned every block
// device and found nothing to boot from. This does not resolve on its own, so
// it is used with [Observer.Never] to fail the spec immediately instead of
// blocking until the guest-agent wait times out.
func HaveNoBootableDevice() Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond := findCondition(vm.Status.Conditions, vmcondition.TypeRunning.String())
		if cond == nil {
			return false, nil
		}
		if cond.Reason == vmcondition.ReasonNoBootableDeviceFound.String() {
			return true, fmt.Errorf("VirtualMachine reports no bootable device: %s", cond.Message)
		}
		return false, nil
	}
}

func findCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}
