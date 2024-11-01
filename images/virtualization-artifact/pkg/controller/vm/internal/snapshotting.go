/*
Copyright 2024 Flant JSC

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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameSnapshottingHandler = "snapshotting"

func NewSnapshottingHandler(client client.Client) *SnapshottingHandler {
	return &SnapshottingHandler{client: client}
}

type SnapshottingHandler struct {
	client client.Client
}

func (h *SnapshottingHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	vm := s.VirtualMachine().Changed()

	if update := addAllUnknown(vm, vmcondition.TypeSnapshotting); update {
		return reconcile.Result{Requeue: true}, nil
	}

	if isDeletion(vm) {
		return reconcile.Result{}, nil
	}

	var vmSnapshots virtv2.VirtualMachineSnapshotList
	err := h.client.List(ctx, &vmSnapshots, client.InNamespace(vm.Namespace))
	if err != nil {
		return reconcile.Result{}, err
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeSnapshotting).Generation(vm.GetGeneration())

	defer func() { conditions.SetCondition(cb, &vm.Status.Conditions) }()

	cb.Status(metav1.ConditionUnknown)

	for _, vmSnapshot := range vmSnapshots.Items {
		if vmSnapshot.Spec.VirtualMachineName != vm.Name {
			continue
		}

		switch vmSnapshot.Status.Phase {
		case virtv2.VirtualMachineSnapshotPhaseReady, virtv2.VirtualMachineSnapshotPhaseTerminating:
			continue
		case virtv2.VirtualMachineSnapshotPhaseInProgress:
			cb.Status(metav1.ConditionTrue).
				Message("The virtual machine is the process of snapshotting.").
				Reason(vmcondition.ReasonSnapshottingInProgress)
			return reconcile.Result{}, nil
		default:
			cb.Status(metav1.ConditionTrue).
				Message("The virtual machine is selected for taking a snapshot.").
				Reason(vmcondition.WaitingForTheSnapshotToStart)
			continue
		}
	}

	return reconcile.Result{}, nil
}

func (h *SnapshottingHandler) Name() string {
	return nameSnapshottingHandler
}
