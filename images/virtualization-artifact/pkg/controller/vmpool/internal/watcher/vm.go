//go:build EE
// +build EE

/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package watcher

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/expectations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// VirtualMachineWatcher watches pool members (VirtualMachines) and, for each
// event, re-enqueues the owning pool and updates its expectations so a lagging
// cache cannot make the pool over-create or over-delete replicas.
type VirtualMachineWatcher struct {
	exp *expectations.Expectations
}

func NewVirtualMachineWatcher(exp *expectations.Expectations) *VirtualMachineWatcher {
	return &VirtualMachineWatcher{exp: exp}
}

func (w *VirtualMachineWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&v1alpha2.VirtualMachine{},
			&memberEventHandler{exp: w.exp},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on pool member VirtualMachines: %w", err)
	}
	return nil
}

// memberEventHandler enqueues the pool that owns a member VM and records
// observed creations/deletions against its expectations.
type memberEventHandler struct {
	exp *expectations.Expectations
}

// ownerKey returns the NamespacedName of the pool that controls vm, or nil if
// the VM is not controlled by a VirtualMachinePool.
func ownerKey(vm *v1alpha2.VirtualMachine) *types.NamespacedName {
	ref := metav1.GetControllerOf(vm)
	if ref == nil || ref.Kind != v1alpha2.VirtualMachinePoolKind || ref.APIVersion != v1alpha2.SchemeGroupVersion.String() {
		return nil
	}
	return &types.NamespacedName{Namespace: vm.GetNamespace(), Name: ref.Name}
}

func (m *memberEventHandler) Create(_ context.Context, e event.TypedCreateEvent[*v1alpha2.VirtualMachine], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	key := ownerKey(e.Object)
	if key == nil {
		return
	}
	m.exp.CreationObserved(key.String())
	q.Add(reconcile.Request{NamespacedName: *key})
}

func (m *memberEventHandler) Delete(_ context.Context, e event.TypedDeleteEvent[*v1alpha2.VirtualMachine], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	key := ownerKey(e.Object)
	if key == nil {
		return
	}
	m.exp.DeletionObserved(key.String(), e.Object.GetUID())
	q.Add(reconcile.Request{NamespacedName: *key})
}

func (m *memberEventHandler) Update(_ context.Context, e event.TypedUpdateEvent[*v1alpha2.VirtualMachine], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	if key := ownerKey(e.ObjectNew); key != nil {
		q.Add(reconcile.Request{NamespacedName: *key})
	}
}

func (m *memberEventHandler) Generic(_ context.Context, e event.TypedGenericEvent[*v1alpha2.VirtualMachine], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	if key := ownerKey(e.Object); key != nil {
		q.Add(reconcile.Request{NamespacedName: *key})
	}
}
