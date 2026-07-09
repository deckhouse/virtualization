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

package vmpoolcondition

// Type is a type of VirtualMachinePool condition.
type Type string

func (t Type) String() string {
	return string(t)
}

const (
	// TypeAvailable indicates whether the pool has enough ready replicas.
	TypeAvailable Type = "Available"
	// TypeProgressing indicates that a self-converging rollout is in progress
	// (scaling, creation, migration).
	TypeProgressing Type = "Progressing"
	// TypeSynced indicates whether every live replica is effectively on the
	// current virtualMachineTemplate.
	TypeSynced Type = "Synced"
)

// AvailableReason is a reason for the Available condition.
type AvailableReason string

func (r AvailableReason) String() string {
	return string(r)
}

const (
	// The pool has no minReplicas/maxUnavailable, so Available means every desired
	// replica is ready — hence "all", not "minimum".
	ReasonAllReplicasReady          AvailableReason = "AllReplicasReady"
	ReasonInsufficientReadyReplicas AvailableReason = "InsufficientReadyReplicas"
)

// ProgressingReason is a reason for the Progressing condition.
type ProgressingReason string

func (r ProgressingReason) String() string {
	return string(r)
}

const (
	ReasonPoolStable ProgressingReason = "PoolStable"
	// ReplicasProgressing covers any convergence of the replica count — scaling
	// as well as replacing a replica that disappeared — not only scaling.
	ReasonReplicasProgressing ProgressingReason = "ReplicasProgressing"
)

// SyncedReason is a reason for the Synced condition.
type SyncedReason string

func (r SyncedReason) String() string {
	return string(r)
}

const (
	ReasonPoolSynced             SyncedReason = "PoolSynced"
	ReasonRolloutInProgress      SyncedReason = "RolloutInProgress"
	ReasonRestartPendingApproval SyncedReason = "RestartPendingApproval"
)
