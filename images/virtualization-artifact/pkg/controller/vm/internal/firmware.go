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

package internal

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameFirmwareHandler = "FirmwareHandler"

func NewFirmwareHandler(image string) *FirmwareHandler {
	return &FirmwareHandler{
		image: image,
	}
}

type FirmwareHandler struct {
	image string
}

func (h *FirmwareHandler) Name() string {
	return nameFirmwareHandler
}

func (h *FirmwareHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	changed := s.VirtualMachine().Changed()
	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	pod, err := s.Pod(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	h.syncFirmwareUpToDate(changed, kvvmi, pod)
	return reconcile.Result{}, nil
}

func (h *FirmwareHandler) syncFirmwareUpToDate(vm *virtv2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, pod *corev1.Pod) {
	if vm == nil {
		return
	}

	upToDate := kvvmi == nil || kvvmi.Status.LauncherContainerImageVersion == "" || kvvmi.Status.LauncherContainerImageVersion == h.image

	cb := conditions.NewConditionBuilder(vmcondition.TypeFirmwareUpToDate).Generation(vm.GetGeneration())
	if pod == nil {
		conditions.RemoveCondition(vmcondition.TypeFirmwareUpToDate, &vm.Status.Conditions)
		return
	}

	if upToDate {
		cb.Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonFirmwareUpToDate).
			Message("")
		conditions.SetCondition(cb, &vm.Status.Conditions)
		return
	}

	cb.Status(metav1.ConditionFalse).
		Reason(vmcondition.ReasonFirmwareOutOfDate).
		Message("The VM firmware version is outdated and not recommended for use with the current version of the virtualization module, please migrate or reboot the VM to upgrade its firmware version.")
	conditions.SetCondition(cb, &vm.Status.Conditions)
}
