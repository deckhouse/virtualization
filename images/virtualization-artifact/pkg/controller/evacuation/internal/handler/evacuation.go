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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
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

func (h *EvacuationHandler) Handle(ctx context.Context, vm *v1alpha2.VirtualMachine) (reconcile.Result, error) {
	if vm == nil {
		return reconcile.Result{}, nil
	}

	if !isVMNeedEvict(vm) {
		return reconcile.Result{}, nil
	}

	// Need retry and starting evacuation if migration failed
	if isVMMigrating(vm) {
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}

	evacuationVMOPs, err := h.getVMOPSByVM(ctx, vm)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Need retry and starting evacuation if migration failed
	if len(evacuationVMOPs) > 0 {
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}

	vmop := newEvacuationVMOP(vm.GetName(), vm.GetNamespace())
	if err = h.client.Create(ctx, vmop); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (h *EvacuationHandler) Name() string {
	return nameDrainHandler
}

func (h *EvacuationHandler) getVMOPSByVM(ctx context.Context, vm *v1alpha2.VirtualMachine) ([]*v1alpha2.VirtualMachineOperation, error) {
	vmops := v1alpha2.VirtualMachineOperationList{}
	err := h.client.List(ctx, &vmops, client.InNamespace(vm.GetNamespace()))
	if err != nil {
		return nil, err
	}

	var evacuationVMOPs []*v1alpha2.VirtualMachineOperation

	for _, vmop := range vmops.Items {
		if vmop.Spec.VirtualMachine != vm.GetName() {
			continue
		}
		if !(vmop.Spec.Type == v1alpha2.VMOPTypeEvict || vmop.Spec.Type == v1alpha2.VMOPTypeMigrate) {
			continue
		}
		if commonvmop.IsFinished(&vmop) {
			continue
		}
		evacuationVMOPs = append(evacuationVMOPs, &vmop)
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

func isVMNeedEvict(vm *v1alpha2.VirtualMachine) bool {
	cond, _ := conditions.GetCondition(vmcondition.TypeNeedsEvict, vm.Status.Conditions)
	return cond.Status == metav1.ConditionTrue
}

func isVMMigrating(vm *v1alpha2.VirtualMachine) bool {
	cond, _ := conditions.GetCondition(vmcondition.TypeMigrating, vm.Status.Conditions)
	return cond.Status == metav1.ConditionTrue
}
