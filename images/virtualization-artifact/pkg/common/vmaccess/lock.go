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

// Package vmaccess holds what the virtual machine access coordinator shares with the other components:
// the lock it keeps on the internal virtual machine, read by the apiserver to reject freeze and
// unfreeze calls that bypass the coordinator.
package vmaccess

import (
	"encoding/json"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

// InternalVirtualMachineGVR addresses the internal virtual machine without kube-api-rewriter, as the
// apiserver sees it.
var InternalVirtualMachineGVR = schema.GroupVersionResource{
	Group:    "internal.virtualization.deckhouse.io",
	Version:  "v1",
	Resource: "internalvirtualizationvirtualmachines",
}

// Lock names the VirtualMachineAccessClaim holding a virtual machine.
type Lock struct {
	// ClaimUID is the UID of the holding claim. Freeze and unfreeze calls carry it as claimUID.
	ClaimUID types.UID `json:"claimUID"`
	// VMIUID is the UID of the virtual machine instance the session is bound to; empty for a stopped
	// virtual machine.
	VMIUID types.UID `json:"vmiUID,omitempty"`
}

// ErrMalformedLock is returned for a lock annotation that names no claim. The coordinator drops such a
// lock, and the apiserver rejects freeze and unfreeze calls until it is gone.
var ErrMalformedLock = errors.New("malformed access lock")

// ReadLock returns the lock recorded on obj, or nil when the virtual machine is not held.
func ReadLock(obj metav1.Object) (*Lock, error) {
	raw, ok := obj.GetAnnotations()[annotations.AnnVMAccessLock]
	if !ok {
		return nil, nil
	}

	var lock Lock
	if err := json.Unmarshal([]byte(raw), &lock); err != nil {
		return nil, fmt.Errorf("%w: parse the %s annotation: %w", ErrMalformedLock, annotations.AnnVMAccessLock, err)
	}

	if lock.ClaimUID == "" {
		return nil, fmt.Errorf("%w: the %s annotation names no claim", ErrMalformedLock, annotations.AnnVMAccessLock)
	}

	return &lock, nil
}

// WriteLock records lock on obj, or removes the record when lock is nil.
func WriteLock(obj metav1.Object, lock *Lock) error {
	anns := obj.GetAnnotations()

	if lock == nil {
		delete(anns, annotations.AnnVMAccessLock)
		obj.SetAnnotations(anns)

		return nil
	}

	raw, err := json.Marshal(lock)
	if err != nil {
		return fmt.Errorf("encode the %s annotation: %w", annotations.AnnVMAccessLock, err)
	}

	if anns == nil {
		anns = make(map[string]string, 1)
	}

	anns[annotations.AnnVMAccessLock] = string(raw)
	obj.SetAnnotations(anns)

	return nil
}

// CheckCall returns an error when a freeze or unfreeze call carrying claimUID may not proceed on a
// virtual machine holding lock. With a lock, only the holder's own call proceeds, so nothing changes the
// guest filesystem under a session. Without a lock any call proceeds: the module itself calls without
// claimUID only to unfreeze a guest a snapshot left frozen before the module update.
func CheckCall(lock *Lock, claimUID string) error {
	if lock == nil {
		return nil
	}

	if claimUID == string(lock.ClaimUID) {
		return nil
	}

	return fmt.Errorf("the virtual machine is held by the VirtualMachineAccessClaim with UID %q", lock.ClaimUID)
}
