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
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const (
	namePeerHandler     = "PeerHandler"
	peerRecoveryRequeue = 5 * time.Second
)

// NewPeerHandler recovers VMs left locked by deleted operations or operations without a finalizer.
func NewPeerHandler(client client.Client, reader client.Reader, recorder eventrecord.EventRecorderLogger) *PeerHandler {
	return &PeerHandler{client: client, reader: reader, recorder: recorder}
}

type PeerHandler struct {
	client client.Client
	// reader is not cached: an operation that has just gone must not stay in a stale cache while the VM is
	// locked by it.
	reader   client.Reader
	recorder eventrecord.EventRecorderLogger
}

func (h *PeerHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	log, ctx := logger.GetHandlerContext(ctx, namePeerHandler)

	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	vm := s.VirtualMachine().Changed()

	kvvm, err := s.KVVM(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("find the internal virtual machine: %w", err)
	}

	op, moved := peerMigrationInProgress(vm, kvvm)
	if !moved {
		return reconcile.Result{}, nil
	}

	// Only the condition locks a VM whose KubeVirt VM is not seen, and the name of the operation is on the
	// KubeVirt VM: any operation that still moves the VM keeps the lock.
	if op == "" {
		vmops, err := s.VMOPs(ctx)
		if err != nil {
			return reconcile.Result{}, err
		}

		for _, vmop := range vmops {
			if commonvmop.PeerName(vmop) != "" && !commonvmop.IsFinished(vmop) {
				return reconcile.Result{}, nil
			}
		}

		log.Warn("No operation moves the virtual machine, releasing it")

		if err = h.setManualPolicy(ctx, s); err != nil {
			return reconcile.Result{}, err
		}

		conditions.RemoveCondition(vmcondition.TypeMovedToPeer, &vm.Status.Conditions)

		return reconcile.Result{}, nil
	}

	// While the operation exists it owns the lock: it finishes the move or undoes it.
	var vmop v1alpha2.VirtualMachineOperation

	err = h.reader.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: op}, &vmop)
	if err == nil {
		return reconcile.Result{}, nil
	}

	if !k8serrors.IsNotFound(err) {
		return reconcile.Result{}, fmt.Errorf("get the operation that moves the virtual machine: %w", err)
	}

	var migration virtv1.VirtualMachineInstanceMigration

	err = h.reader.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: commonvmop.MigrationName(op)}, &migration)
	if client.IgnoreNotFound(err) != nil {
		return reconcile.Result{}, fmt.Errorf("find the migration of the missing operation: %w", err)
	}

	if err == nil && !migration.IsFinal() {
		if migration.DeletionTimestamp.IsZero() {
			if err := h.client.Delete(ctx, &migration); client.IgnoreNotFound(err) != nil {
				return reconcile.Result{}, fmt.Errorf("cancel the migration of the missing operation: %w", err)
			}
		}

		return reconcile.Result{RequeueAfter: peerRecoveryRequeue}, nil
	}

	log.Warn("The operation that moves the virtual machine is gone, releasing the virtual machine", "operation", op)

	var kvvmi virtv1.VirtualMachineInstance

	err = h.reader.Get(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, &kvvmi)
	if client.IgnoreNotFound(err) != nil {
		return reconcile.Result{}, fmt.Errorf("find the internal virtual machine instance: %w", err)
	}
	// A completed source migration may already run in the peer even while the local cache still says Running.
	if err != nil || kvvmi.Status.Phase != virtv1.Running || migration.Status.Phase == virtv1.MigrationSucceeded {
		if err = h.setManualPolicy(ctx, s); err != nil {
			return reconcile.Result{}, err
		}
	}

	patch, _ := json.Marshal(map[string]any{
		"metadata": map[string]any{"annotations": map[string]any{annotations.AnnPeerMigrationVMOP: nil}},
	})
	if err = h.client.Patch(ctx, kvvm, client.RawPatch(types.MergePatchType, patch)); err != nil {
		return reconcile.Result{}, fmt.Errorf("release the internal virtual machine: %w", err)
	}

	h.recorder.Event(vm, corev1.EventTypeWarning, v1alpha2.ReasonErrVMOPFailed,
		fmt.Sprintf("The VirtualMachineOperation %q that moved the virtual machine is gone. The virtual machine is released: check the peer cluster it was moved to before starting it here.", op))

	return reconcile.Result{}, nil
}

// setManualPolicy keeps a VM whose guest may run in the peer from being started here by its run policy.
// The changed VM takes the new version, or the status update at the end of the reconcile would conflict
// with the patch.
func (h *PeerHandler) setManualPolicy(ctx context.Context, s state.VirtualMachineState) error {
	changed := s.VirtualMachine().Changed()
	if changed.Spec.RunPolicy == v1alpha2.ManualPolicy {
		return nil
	}

	patched := s.VirtualMachine().Current().DeepCopy()

	patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"runPolicy": v1alpha2.ManualPolicy}})
	if err := h.client.Patch(ctx, patched, client.RawPatch(types.MergePatchType, patch)); err != nil {
		return fmt.Errorf("hand the virtual machine back stopped: %w", err)
	}

	changed.Spec.RunPolicy = v1alpha2.ManualPolicy
	changed.ResourceVersion = patched.ResourceVersion

	return nil
}

func (h *PeerHandler) Name() string {
	return namePeerHandler
}
