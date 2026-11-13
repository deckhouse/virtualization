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

// Package state loads everything the virtual machine access coordinator decides on for one virtual
// machine, and writes back the claims and the lock.
package state

import (
	"context"
	"errors"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/vmaccess"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	coordinationv1alpha1 "github.com/deckhouse/virtualization/api/coordination/v1alpha1"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// Claim is a VirtualMachineAccessClaim as loaded and as the handlers want it to be.
type Claim struct {
	Current *coordinationv1alpha1.VirtualMachineAccessClaim
	Changed *coordinationv1alpha1.VirtualMachineAccessClaim
}

// State is the virtual machine the coordinator reconciles, with its claims and its lock.
type State struct {
	client client.Client
	key    types.NamespacedName

	VM    *v1alpha2.VirtualMachine
	KVVM  *virtv1.VirtualMachine
	KVVMI *virtv1.VirtualMachineInstance
	// Migration is a migration of the virtual machine that has not finished yet, if any.
	Migration *virtv1.VirtualMachineInstanceMigration
	// Claims are ordered by creation, so the earlier claim is decided first.
	Claims []*Claim
	// Lock is the lock recorded on KVVM, or nil when the virtual machine is not held.
	Lock *vmaccess.Lock
	// MalformedLock reports a lock annotation on KVVM that names no claim. Lock is nil then, and
	// ReleaseLock removes the annotation.
	MalformedLock bool
}

func New(c client.Client, key types.NamespacedName) *State {
	return &State{client: c, key: key}
}

func (s *State) Key() types.NamespacedName {
	return s.key
}

// Load reads the virtual machine, its internal objects, its unfinished migration and its claims.
func (s *State) Load(ctx context.Context) error {
	var err error

	if s.VM, err = get(ctx, s.client, s.key, &v1alpha2.VirtualMachine{}); err != nil {
		return err
	}

	if s.KVVM, err = get(ctx, s.client, s.key, &virtv1.VirtualMachine{}); err != nil {
		return err
	}

	if s.KVVMI, err = get(ctx, s.client, s.key, &virtv1.VirtualMachineInstance{}); err != nil {
		return err
	}

	s.Lock, s.MalformedLock = nil, false
	if s.KVVM != nil {
		s.Lock, err = vmaccess.ReadLock(s.KVVM)
		switch {
		case errors.Is(err, vmaccess.ErrMalformedLock):
			s.MalformedLock = true
		case err != nil:
			return err
		}
	}

	var migrations virtv1.VirtualMachineInstanceMigrationList
	if err = s.client.List(ctx, &migrations,
		client.InNamespace(s.key.Namespace),
		client.MatchingFields{indexer.IndexFieldVMIMByVMI: s.key.Name},
	); err != nil {
		return fmt.Errorf("list the migrations of the virtual machine: %w", err)
	}

	s.Migration = nil

	for i := range migrations.Items {
		if !isFinalMigration(&migrations.Items[i]) {
			s.Migration = &migrations.Items[i]
			break
		}
	}

	var claims coordinationv1alpha1.VirtualMachineAccessClaimList
	if err = s.client.List(ctx, &claims,
		client.InNamespace(s.key.Namespace),
		client.MatchingFields{indexer.IndexFieldVMACByVM: s.key.Name},
	); err != nil {
		return fmt.Errorf("list the access claims of the virtual machine: %w", err)
	}

	sort.SliceStable(claims.Items, func(i, j int) bool {
		a, b := claims.Items[i].CreationTimestamp, claims.Items[j].CreationTimestamp
		if !a.Equal(&b) {
			return a.Before(&b)
		}

		return claims.Items[i].Name < claims.Items[j].Name
	})

	s.Claims = make([]*Claim, 0, len(claims.Items))
	for i := range claims.Items {
		s.Claims = append(s.Claims, &Claim{
			Current: claims.Items[i].DeepCopy(),
			Changed: claims.Items[i].DeepCopy(),
		})
	}

	return nil
}

// Holder returns the claim the lock names, or nil when there is no lock or its claim is gone.
func (s *State) Holder() *Claim {
	if s.Lock == nil {
		return nil
	}
	for _, c := range s.Claims {
		if c.Current.UID == s.Lock.ClaimUID {
			return c
		}
	}

	return nil
}

// AcquireLock records claim as the holder on the internal virtual machine.
//
// The write carries the resourceVersion the lock was read at, so two decisions made from the same stale
// cache cannot both succeed: the second one gets a conflict and is retried against a fresh read.
func (s *State) AcquireLock(ctx context.Context, claimUID, vmiUID types.UID) error {
	lock := &vmaccess.Lock{ClaimUID: claimUID, VMIUID: vmiUID}
	if err := s.patchLock(ctx, lock); err != nil {
		return err
	}

	s.Lock = lock

	return nil
}

// ReleaseLock removes the lock from the internal virtual machine.
func (s *State) ReleaseLock(ctx context.Context) error {
	if s.Lock == nil && !s.MalformedLock {
		return nil
	}
	if err := s.patchLock(ctx, nil); err != nil {
		return err
	}

	s.Lock, s.MalformedLock = nil, false

	return nil
}

func (s *State) patchLock(ctx context.Context, lock *vmaccess.Lock) error {
	if s.KVVM == nil {
		return errors.New("the internal virtual machine does not exist")
	}

	base := s.KVVM.DeepCopy()
	if err := vmaccess.WriteLock(s.KVVM, lock); err != nil {
		return err
	}

	if err := s.client.Patch(ctx, s.KVVM, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
		s.KVVM = base
		return fmt.Errorf("write the access lock: %w", err)
	}

	return nil
}

// PatchClaims writes the finalizers and status the handlers changed.
func (s *State) PatchClaims(ctx context.Context) error {
	var errs error

	for _, c := range s.Claims {
		if !equality.Semantic.DeepEqual(c.Current.Status, c.Changed.Status) {
			patched := c.Current.DeepCopy()
			patched.Status = c.Changed.Status
			// A status patched from a stale read would overwrite a verdict written meanwhile; the optimistic
			// lock turns that into a conflict and a retry.
			err := s.client.Status().Patch(ctx, patched, client.MergeFromWithOptions(c.Current, client.MergeFromWithOptimisticLock{}))
			if err != nil {
				if !apierrors.IsNotFound(err) {
					errs = errors.Join(errs, fmt.Errorf("patch the status of the access claim %q: %w", c.Current.Name, err))
				}

				continue
			}
			// The response carries the new resourceVersion, which the finalizer patch below is locked on.
			c.Current = patched
		}

		if !equality.Semantic.DeepEqual(c.Current.Finalizers, c.Changed.Finalizers) {
			patched := c.Current.DeepCopy()
			patched.Finalizers = c.Changed.Finalizers
			// A finalizer list is replaced as a whole by a merge patch, so a patch from a stale read would drop
			// a finalizer added meanwhile; the optimistic lock turns that into a conflict and a retry.
			err := s.client.Patch(ctx, patched, client.MergeFromWithOptions(c.Current, client.MergeFromWithOptimisticLock{}))
			if err != nil && !apierrors.IsNotFound(err) {
				errs = errors.Join(errs, fmt.Errorf("patch the finalizers of the access claim %q: %w", c.Current.Name, err))
			}
		}
	}

	return errs
}

func get[T client.Object](ctx context.Context, c client.Client, key types.NamespacedName, obj T) (T, error) {
	var empty T

	if err := c.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return empty, nil
		}

		return empty, fmt.Errorf("get %T %s: %w", obj, key, err)
	}

	return obj, nil
}

func isFinalMigration(m *virtv1.VirtualMachineInstanceMigration) bool {
	return m.Status.Phase == virtv1.MigrationFailed || m.Status.Phase == virtv1.MigrationSucceeded
}

// AgentConnected reports whether the guest agent of the instance is connected, which a freeze needs.
func AgentConnected(kvvmi *virtv1.VirtualMachineInstance) bool {
	for _, c := range kvvmi.Status.Conditions {
		if c.Type == virtv1.VirtualMachineInstanceAgentConnected {
			return c.Status == corev1.ConditionTrue
		}
	}

	return false
}
