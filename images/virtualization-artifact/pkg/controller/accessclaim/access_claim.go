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

// Package accessclaim is how the snapshot controllers ask the virtual machine access coordinator
// for access: they create a VirtualMachineAccessClaim, wait for its decision, release it once the
// snapshot no longer needs the virtual machine, and read the verdict.
package accessclaim

import (
	"context"
	"fmt"
	"slices"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/coordination/v1alpha1/vmaccondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// Name returns the name of the claim a snapshot creates. It is derived from the snapshot UID, so a
// snapshot recreated under the same name never picks up the claim of its predecessor.
func Name(snapshot client.Object) string {
	switch snapshot.(type) {
	case *v1alpha2.VirtualDiskSnapshot:
		return "vds-" + string(snapshot.GetUID())
	default:
		return "vms-" + string(snapshot.GetUID())
	}
}

// Timeout is the session timeout a snapshot asks the coordinator for. It is a minute longer than the planning
// and children settle deadlines of a unified VirtualMachineSnapshot together, so those fire first with a
// message that says what was waited on. Built-in snapshots use the same value.
const Timeout = 13 * time.Minute

// Get returns the claim of the snapshot, or nil when it does not exist. It reads the cache first and asks
// apiReader only when the cache has no claim: a claim created moments ago may not be in the cache yet, and
// reading it as absent would take the snapshot for one started before the module update.
func Get(ctx context.Context, cache, apiReader client.Reader, namespace, name string) (*coordinationv1alpha1.VirtualMachineAccessClaim, error) {
	return get(ctx, []client.Reader{cache, apiReader}, namespace, name)
}

// GetCached returns the claim of the snapshot from the cache alone, or nil when the cache has none. It is
// for a snapshot that does not act on an absent claim: the claim watch brings a claim the cache does not
// have yet.
func GetCached(ctx context.Context, cache client.Reader, namespace, name string) (*coordinationv1alpha1.VirtualMachineAccessClaim, error) {
	return get(ctx, []client.Reader{cache}, namespace, name)
}

func get(ctx context.Context, readers []client.Reader, namespace, name string) (*coordinationv1alpha1.VirtualMachineAccessClaim, error) {
	for _, reader := range readers {
		claim := &coordinationv1alpha1.VirtualMachineAccessClaim{}

		err := reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, claim)
		switch {
		case err == nil:
			return claim, nil
		case !apierrors.IsNotFound(err):
			return nil, fmt.Errorf("get the access claim %q: %w", name, err)
		}
	}

	return nil, nil
}

// Create creates the claim of snapshot for vm, owned by snapshot, and returns it. A claim that already
// exists is returned as is. A snapshot with requiredConsistency asks for the capture under a freeze that
// must succeed; any other snapshot asks for the capture alone.
func Create(ctx context.Context, c client.Client, apiReader client.Reader, snapshot client.Object, vm *v1alpha2.VirtualMachine, requiredConsistency bool) (*coordinationv1alpha1.VirtualMachineAccessClaim, error) {
	modes := []coordinationv1alpha1.AccessMode{coordinationv1alpha1.AccessModeSnapshot}

	var policy *coordinationv1alpha1.FreezePolicy

	if requiredConsistency {
		modes = append(modes, coordinationv1alpha1.AccessModeFreeze)
		policy = ptr.To(coordinationv1alpha1.FreezePolicyRequired)
	}

	claim := &coordinationv1alpha1.VirtualMachineAccessClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            Name(snapshot),
			Namespace:       snapshot.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(snapshot, snapshotKind(snapshot))},
		},
		Spec: coordinationv1alpha1.VirtualMachineAccessClaimSpec{
			VirtualMachine: coordinationv1alpha1.VirtualMachineAccessClaimTarget{
				Name: vm.Name,
				UID:  vm.UID,
			},
			Access:       modes,
			FreezePolicy: policy,
			Timeout:      metav1.Duration{Duration: Timeout},
		},
	}

	err := c.Create(ctx, claim)
	switch {
	case err == nil:
		return claim, nil
	case apierrors.IsAlreadyExists(err):
		return Get(ctx, c, apiReader, claim.Namespace, claim.Name)
	default:
		return nil, fmt.Errorf("create the access claim %q: %w", claim.Name, err)
	}
}

func snapshotKind(snapshot client.Object) schema.GroupVersionKind {
	switch snapshot.(type) {
	case *v1alpha2.VirtualDiskSnapshot:
		return v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualDiskSnapshotKind)
	default:
		return v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineSnapshotKind)
	}
}

// Release tells the coordinator the snapshot no longer needs the virtual machine. It is a no-op for a
// claim already released or decided against.
func Release(ctx context.Context, c client.Client, claim *coordinationv1alpha1.VirtualMachineAccessClaim) error {
	if claim == nil || claim.Spec.Release {
		return nil
	}

	switch claim.Status.Phase {
	case coordinationv1alpha1.AccessClaimPhaseDenied, coordinationv1alpha1.AccessClaimPhaseAborted:
		return nil
	}

	patch := client.RawPatch(types.MergePatchType, []byte(`{"spec":{"release":true}}`))
	if err := c.Patch(ctx, claim, patch); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("release the access claim %q: %w", claim.Name, err)
	}

	claim.Spec.Release = true

	return nil
}

// IsGranted reports whether the snapshot may capture the virtual machine.
func IsGranted(claim *coordinationv1alpha1.VirtualMachineAccessClaim) bool {
	return claim != nil && claim.Status.Phase == coordinationv1alpha1.AccessClaimPhaseGranted
}

// IsRefused reports whether the claim was denied or its session aborted.
func IsRefused(claim *coordinationv1alpha1.VirtualMachineAccessClaim) bool {
	return claim != nil && (claim.Status.Phase == coordinationv1alpha1.AccessClaimPhaseDenied ||
		claim.Status.Phase == coordinationv1alpha1.AccessClaimPhaseAborted)
}

// IsCompleted reports whether the session went through without violations.
func IsCompleted(claim *coordinationv1alpha1.VirtualMachineAccessClaim) bool {
	return claim != nil && claim.Status.Phase == coordinationv1alpha1.AccessClaimPhaseCompleted
}

// IsUnfreezing reports a session whose guest the coordinator is still unfreezing. The coordinator writes the
// verdict before the unfreeze goes through, so a completed claim may still be unfreezing.
func IsUnfreezing(claim *coordinationv1alpha1.VirtualMachineAccessClaim) bool {
	c := unfrozen(claim)
	return c != nil && c.Reason == vmaccondition.Unfreezing.String()
}

// UnfreezeNotConfirmed returns the coordinator's message when the guest did not confirm the unfreeze at the
// end of the session, and reports whether it did not. Such a guest stays frozen until its own safety
// unfreeze, which the snapshot tells the user about.
func UnfreezeNotConfirmed(claim *coordinationv1alpha1.VirtualMachineAccessClaim) (string, bool) {
	c := unfrozen(claim)
	if c == nil || c.Reason != vmaccondition.UnfreezeNotConfirmed.String() {
		return "", false
	}

	return c.Message, true
}

func unfrozen(claim *coordinationv1alpha1.VirtualMachineAccessClaim) *metav1.Condition {
	if claim == nil {
		return nil
	}

	return meta.FindStatusCondition(claim.Status.Conditions, vmaccondition.UnfrozenType.String())
}

// Consistent reports whether a snapshot taken under the claim is consistent: the session was granted and
// completed with the guest frozen or the virtual machine stopped throughout. grantedAt tells a session on a
// stopped machine from a claim released before it was granted, as neither records an instance.
func Consistent(claim *coordinationv1alpha1.VirtualMachineAccessClaim) bool {
	return IsCompleted(claim) && claim.Status.GrantedAt != nil &&
		(claim.Status.Frozen || claim.Status.VirtualMachineInstanceUID == "")
}

// GrantedCondition returns the reason and message of the Granted condition. The coordinator writes the
// message for the snapshot, which copies both into its own status.
func GrantedCondition(claim *coordinationv1alpha1.VirtualMachineAccessClaim) (reason, message string) {
	if claim == nil {
		return "", ""
	}

	if c := meta.FindStatusCondition(claim.Status.Conditions, vmaccondition.GrantedType.String()); c != nil {
		return c.Reason, c.Message
	}

	return "", ""
}

// The reasons both VirtualMachineSnapshot and VirtualDiskSnapshot declare for a snapshot that waits for
// its access.
const (
	reasonFileSystemFreezing = "FileSystemFreezing"
	reasonSnapshotting       = "Snapshotting"
)

// Waiting returns the reason and message a snapshot shows while its claim waits for a decision. A claim
// that asks for a freeze waits under FileSystemFreezing, with the coordinator's message once it is freezing
// the guest; one without a freeze waits under Snapshotting.
func Waiting(claim *coordinationv1alpha1.VirtualMachineAccessClaim) (reason, message string) {
	if !slices.Contains(claim.Spec.Access, coordinationv1alpha1.AccessModeFreeze) {
		return reasonSnapshotting, msgWaiting(claim)
	}

	if reason, message := GrantedCondition(claim); reason == vmaccondition.Freezing.String() {
		return reasonFileSystemFreezing, message
	}

	return reasonFileSystemFreezing, msgWaiting(claim)
}

func msgWaiting(claim *coordinationv1alpha1.VirtualMachineAccessClaim) string {
	return fmt.Sprintf("Waiting for access to the virtual machine %q.", claim.Spec.VirtualMachine.Name)
}
