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

package internal

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The messages of the Granted condition are written for the snapshot that owns the claim: the snapshot
// copies them into its own condition, so they speak of the virtual machine and its snapshot.

func msgFreezing(vm string) string {
	return fmt.Sprintf("Waiting for the guest of the virtual machine %q to confirm the filesystem freeze.", vm)
}

func msgFrozen(vm string) string {
	return fmt.Sprintf("The filesystem of the virtual machine %q is frozen.", vm)
}

func msgNotRequiredStopped(vm string) string {
	return fmt.Sprintf("The virtual machine %q is stopped, so its filesystem does not need to be frozen.", vm)
}

func msgNotRequiredNotRequested(vm string) string {
	return fmt.Sprintf("The access to the virtual machine %q is granted without a filesystem freeze, as none was requested.", vm)
}

func msgFreezeSkipped(vm string) string {
	return fmt.Sprintf("The filesystem of the virtual machine %q cannot be frozen: the guest agent is not connected. The snapshot is taken without a freeze and is not consistent.", vm)
}

func msgBusy(vm string, holder *metav1.OwnerReference) string {
	if holder == nil {
		return fmt.Sprintf("The virtual machine %q is in use by another snapshot. Retry after it completes.", vm)
	}

	return fmt.Sprintf("The virtual machine %q is in use by the %s %q. Retry after it completes.", vm, holder.Kind, holder.Name)
}

func msgMigrating(vm string, operation *metav1.OwnerReference) string {
	if operation == nil {
		return fmt.Sprintf("The virtual machine %q is migrating. Retry after the migration completes.", vm)
	}

	return fmt.Sprintf("The virtual machine %q is being migrated by the %s %q. Retry after the migration completes.", vm, operation.Kind, operation.Name)
}

func msgAlreadyFrozen(vm string) string {
	return fmt.Sprintf("Cannot take a snapshot of the virtual machine %q: its filesystem is already frozen. Another snapshot of this virtual machine may still be in progress. Retry after the virtual machine filesystem is unfrozen.", vm)
}

func msgVMNotFound(vm string) string {
	return fmt.Sprintf("The virtual machine %q does not exist.", vm)
}

func msgVMNotReady(vm string) string {
	return fmt.Sprintf("The virtual machine %q is not ready to be snapshotted yet. Retry later.", vm)
}

func msgVMStartingOrStopping(vm string) string {
	return fmt.Sprintf("The virtual machine %q is starting or stopping. Retry after it is running or stopped.", vm)
}

func msgAgentNotConnected(vm string) string {
	return fmt.Sprintf("Cannot freeze the filesystem of the virtual machine %q: the guest agent is not connected.", vm)
}

func msgFreezeNotConfirmed(vm string, timeout time.Duration) string {
	return fmt.Sprintf("The guest of the virtual machine %q did not confirm the filesystem freeze within %s.", vm, timeout)
}

func msgVMRecreated(vm string) string {
	return fmt.Sprintf("The virtual machine %q was deleted or recreated while the snapshot was being taken.", vm)
}

func msgVMInstanceChanged(vm string) string {
	return fmt.Sprintf("The virtual machine %q was started, stopped or restarted while the snapshot was being taken.", vm)
}

func msgMigrationStarted(vm string) string {
	return fmt.Sprintf("The virtual machine %q started migrating while the snapshot was being taken.", vm)
}

func msgUnfrozenUnexpectedly(vm string) string {
	return fmt.Sprintf("The filesystem of the virtual machine %q was unfrozen while the snapshot was being taken.", vm)
}

func msgLockLost(vm string) string {
	return fmt.Sprintf("The state of the virtual machine %q changed while the snapshot was being taken. Take a new snapshot.", vm)
}

func msgDeadlineExceeded(vm string, timeout time.Duration) string {
	return fmt.Sprintf("The snapshot of the virtual machine %q was not taken within %s, and the access to the virtual machine was withdrawn.", vm, timeout)
}

func msgReleased(vm string) string {
	return fmt.Sprintf("The access to the virtual machine %q was released.", vm)
}

func msgUnfreezing(vm string) string {
	return fmt.Sprintf("Waiting for the guest of the virtual machine %q to confirm the filesystem unfreeze.", vm)
}

func msgUnfreezeConfirmed(vm string) string {
	return fmt.Sprintf("The filesystem of the virtual machine %q is unfrozen.", vm)
}

func msgUnfreezeNotConfirmed(vm string, timeout time.Duration) string {
	return fmt.Sprintf("The guest of the virtual machine %q did not confirm the filesystem unfreeze within %s. The guest unfreezes the filesystem on its own once the safety timeout expires.", vm, timeout)
}

// The messages of the events on the virtual machine.

func msgStaleFreezeNotUnfrozen(vm string, err error) string {
	return fmt.Sprintf("Failed to unfreeze the virtual machine %q, which a snapshot taken before the virtualization module update may have left frozen: %v.", vm, err)
}

func msgStaleFreezeRequestNotRemoved(vm string, err error) string {
	return fmt.Sprintf("Failed to clear the filesystem freeze request a snapshot taken before the virtualization module update left on the virtual machine %q: %v.", vm, err)
}
