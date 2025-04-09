/*
Copyright 2025 Flant JSC

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

package handler

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/evacuation/internal/taint"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameDrainHandler = "EvacuationHandler"

func NewEvacuationHandler(client client.Client) *EvacuationHandler {
	return &EvacuationHandler{
		client: client,
	}
}

type EvacuationHandler struct {
	client client.Client
}

func (h *EvacuationHandler) Handle(ctx context.Context, node *corev1.Node) (reconcile.Result, error) {
	if node == nil {
		return reconcile.Result{}, nil
	}

	vms, err := h.listVirtualMachineByNode(ctx, node.GetName())
	if err != nil {
		return reconcile.Result{}, err
	}

	vmsToMigrate := filterVirtualMachinesToMigrate(node, vms)
	if len(vmsToMigrate) == 0 {
		return reconcile.Result{}, nil
	}

	evacuationVMOPs, err := h.getVMOPSByVM(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	for _, vm := range vmsToMigrate {
		if isVMMigrating(vm) || h.vmopInProgressOrPendingOrTerminatingExists(client.ObjectKeyFromObject(vm), evacuationVMOPs) {
			continue
		}

		vmop := newEvacuationVMOP(vm.GetName(), vm.GetNamespace())
		if err = h.client.Create(ctx, vmop); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (h *EvacuationHandler) Name() string {
	return nameDrainHandler
}

func (h *EvacuationHandler) vmopInProgressOrPendingOrTerminatingExists(vmKey client.ObjectKey, evacuationVMOPs map[client.ObjectKey][]*v1alpha2.VirtualMachineOperation) bool {
	for _, vmop := range evacuationVMOPs[vmKey] {
		if commonvmop.IsInProgressOrPending(vmop) || vmop.Status.Phase == v1alpha2.VMOPPhaseTerminating {
			return true
		}
	}
	return false
}

func (h *EvacuationHandler) listVirtualMachineByNode(ctx context.Context, nodeName string) ([]*v1alpha2.VirtualMachine, error) {
	vms := &v1alpha2.VirtualMachineList{}
	err := h.client.List(ctx, vms, client.MatchingFields{
		indexer.IndexFieldVMByNode: nodeName,
	})
	if err != nil {
		return nil, err
	}
	result := make([]*v1alpha2.VirtualMachine, len(vms.Items))
	for i := range vms.Items {
		result[i] = &vms.Items[i]
	}
	return result, nil
}

func (h *EvacuationHandler) getVMOPSByVM(ctx context.Context) (map[client.ObjectKey][]*v1alpha2.VirtualMachineOperation, error) {
	vmops := v1alpha2.VirtualMachineOperationList{}
	err := h.client.List(ctx, &vmops)
	if err != nil {
		return nil, err
	}

	evacuationVMOPs := make(map[client.ObjectKey][]*v1alpha2.VirtualMachineOperation)

	for i := range vmops.Items {
		vmop := &vmops.Items[i]
		if !(vmop.Spec.Type == v1alpha2.VMOPTypeEvict || vmop.Spec.Type == v1alpha2.VMOPTypeMigrate) {
			continue
		}
		key := client.ObjectKey{Name: vmop.Spec.VirtualMachine, Namespace: vmop.Namespace}
		evacuationVMOPs[key] = append(evacuationVMOPs[key], vmop)
	}
	return evacuationVMOPs, nil
}

func newEvacuationVMOP(vmName, namespace string) *v1alpha2.VirtualMachineOperation {
	return vmopbuilder.New(
		vmopbuilder.WithGenerateName("evacuation-"),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithAnnotation(annotations.AnnVMOPEvacuation, "true"),
		vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
		vmopbuilder.WithVirtualMachine(vmName),
	)
}

func filterVirtualMachinesToMigrate(node *corev1.Node, vms []*v1alpha2.VirtualMachine) []*v1alpha2.VirtualMachine {
	if taint.IsTaintedToDrain(node) {
		return vms
	}
	if evictVms := filterVirtualMachinesWithNeedsEvict(vms); len(evictVms) > 0 {
		return evictVms
	}

	return nil
}

func filterVirtualMachinesWithNeedsEvict(vms []*v1alpha2.VirtualMachine) []*v1alpha2.VirtualMachine {
	var needEvict []*v1alpha2.VirtualMachine
	for i := range vms {
		if isVMNeedEvict(vms[i]) {
			needEvict = append(needEvict, vms[i])
		}
	}
	return needEvict
}

func isVMNeedEvict(vm *v1alpha2.VirtualMachine) bool {
	cond, _ := conditions.GetCondition(vmcondition.TypeNeedsEvict, vm.Status.Conditions)
	return cond.Status == metav1.ConditionTrue
}

func isVMMigrating(vm *v1alpha2.VirtualMachine) bool {
	cond, _ := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	return cond.Status == metav1.ConditionTrue
}
