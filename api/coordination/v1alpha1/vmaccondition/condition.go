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

package vmaccondition

// Type represents the various condition types for the `VirtualMachineAccessClaim`.
type Type string

func (t Type) String() string {
	return string(t)
}

const (
	// GrantedType indicates whether the access is granted. Its reason tells the requester what to do next.
	GrantedType Type = "Granted"
	// UnfrozenType indicates whether the guest confirmed the unfreeze of a session that froze the filesystem.
	UnfrozenType Type = "Unfrozen"
)

// GrantedReason represents the various reasons for the `Granted` condition type.
type GrantedReason string

func (r GrantedReason) String() string {
	return string(r)
}

const (
	// Freezing signifies that the coordinator is granting the access and waits for the guest to confirm the filesystem freeze.
	Freezing GrantedReason = "Freezing"
	// Frozen signifies that the access is granted with the virtual machine filesystem frozen.
	Frozen GrantedReason = "Frozen"
	// NotRequired signifies that the access is granted without a freeze because the virtual machine is stopped or the freeze was not requested.
	NotRequired GrantedReason = "NotRequired"
	// FreezeSkipped signifies that the filesystem could not be frozen and the access is granted without the freeze, as the BestEffort policy allows.
	FreezeSkipped GrantedReason = "FreezeSkipped"

	// VirtualMachineBusy signifies that the access is denied because another session holds the virtual machine.
	VirtualMachineBusy GrantedReason = "VirtualMachineBusy"
	// FilesystemAlreadyFrozen signifies that the access is denied because the virtual machine filesystem is already frozen outside a coordinator session.
	FilesystemAlreadyFrozen GrantedReason = "FilesystemAlreadyFrozen"
	// VirtualMachineNotReady signifies that the access is denied because the virtual machine is not ready to be accessed: it was just created, or its internal state was removed.
	VirtualMachineNotReady GrantedReason = "VirtualMachineNotReady"
	// FilesystemFreezeFailed signifies that the access is denied because the filesystem could not be frozen and the Required policy forbids proceeding without the freeze.
	FilesystemFreezeFailed GrantedReason = "FilesystemFreezeFailed"

	// VirtualMachineStateChanged signifies that the claim was denied or its session aborted because the virtual machine stopped, started, restarted, started migrating or was recreated, or because the lock the claim took on the virtual machine was lost.
	VirtualMachineStateChanged GrantedReason = "VirtualMachineStateChanged"
	// FilesystemUnfrozenUnexpectedly signifies that the session was aborted because the filesystem was unfrozen without a coordinator decision.
	FilesystemUnfrozenUnexpectedly GrantedReason = "FilesystemUnfrozenUnexpectedly"
	// DeadlineExceeded signifies that the session was aborted because its timeout expired and the coordinator unfroze the virtual machine.
	DeadlineExceeded GrantedReason = "DeadlineExceeded"

	// Released signifies that the access was released and the session went through without violations.
	Released GrantedReason = "Released"
)

// UnfrozenReason represents the various reasons for the `Unfrozen` condition type.
type UnfrozenReason string

func (r UnfrozenReason) String() string {
	return string(r)
}

const (
	// Unfreezing signifies that the coordinator requested the unfreeze and waits for the guest to confirm it.
	Unfreezing UnfrozenReason = "Unfreezing"
	// UnfreezeConfirmed signifies that the guest confirmed the unfreeze.
	UnfreezeConfirmed UnfrozenReason = "UnfreezeConfirmed"
	// UnfreezeNotConfirmed signifies that the guest did not confirm the unfreeze in time; the safety unfreeze in the guest is left to thaw it.
	UnfreezeNotConfirmed UnfrozenReason = "UnfreezeNotConfirmed"
)
