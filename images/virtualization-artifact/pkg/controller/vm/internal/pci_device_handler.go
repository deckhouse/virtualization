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
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/pcidevicecondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const namePCIDeviceHandler = "PCIDeviceHandler"

func NewPCIDeviceHandler(cl client.Client) *PCIDeviceHandler {
	return &PCIDeviceHandler{client: cl}
}

// PCIDeviceHandler builds vm.status.pciDevices. PCI devices are cold-plugged:
// the KVVM builder wires them through DRA resource claims at pod creation, so
// this handler only reflects readiness and runtime attachment state.
type PCIDeviceHandler struct {
	client client.Client
}

func (h *PCIDeviceHandler) Name() string {
	return namePCIDeviceHandler
}

func (h *PCIDeviceHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	vm := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if len(vm.Spec.PCIDevices) == 0 {
		changed.Status.PCIDevices = nil
		return reconcile.Result{}, nil
	}

	pciDevicesByName, err := s.PCIDevicesByName(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get PCI devices: %w", err)
	}

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get KVVMI: %w", err)
	}
	attachedByName := attachedHostDevicesByName(kvvmi)

	nextStatusRefs := make([]v1alpha2.PCIDeviceStatusRef, 0, len(vm.Spec.PCIDevices))
	var notReady []string
	for _, ref := range vm.Spec.PCIDevices {
		_, attached := attachedByName[ref.Name]
		ready := isPCIDeviceReady(pciDevicesByName[ref.Name])
		if !ready {
			notReady = append(notReady, describeNotReadyPCIDevice(ref.Name, pciDevicesByName[ref.Name]))
		}
		nextStatusRefs = append(nextStatusRefs, v1alpha2.PCIDeviceStatusRef{
			Name:     ref.Name,
			Attached: attached,
			Ready:    ready,
		})
	}

	changed.Status.PCIDevices = nextStatusRefs

	cb := conditions.NewConditionBuilder(vmcondition.TypePCIDevicesReady).Generation(changed.Generation)
	if len(notReady) == 0 {
		cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonPCIDevicesReady).Message("All PCI devices are ready.")
	} else {
		cb.Status(metav1.ConditionFalse).Reason(vmcondition.ReasonPCIDevicesNotReady).Message(strings.Join(notReady, " "))
	}
	conditions.SetCondition(cb, &changed.Status.Conditions)

	return reconcile.Result{}, nil
}

// describeNotReadyPCIDevice explains why a referenced PCIDevice blocks the
// virtual machine start.
func describeNotReadyPCIDevice(name string, pciDevice *v1alpha2.PCIDevice) string {
	switch {
	case pciDevice == nil:
		return fmt.Sprintf("PCIDevice %q was not found in the namespace; check the name or ask the administrator to assign the device to this namespace.", name)
	case !pciDevice.GetDeletionTimestamp().IsZero():
		return fmt.Sprintf("PCIDevice %q is being deleted; remove it from the virtual machine specification or ask the administrator to assign it again.", name)
	default:
		readyCondition, found := conditions.GetCondition(pcidevicecondition.ReadyType, pciDevice.Status.Conditions)
		if found && readyCondition.Message != "" {
			return fmt.Sprintf("PCIDevice %q is not ready: %s", name, readyCondition.Message)
		}
		return fmt.Sprintf("PCIDevice %q is not ready yet.", name)
	}
}

func isPCIDeviceReady(pciDevice *v1alpha2.PCIDevice) bool {
	if pciDevice == nil || !pciDevice.GetDeletionTimestamp().IsZero() {
		return false
	}
	if pciDevice.Status.Attributes.PCIAddress == "" || pciDevice.Status.NodeName == "" {
		return false
	}
	readyCondition, found := conditions.GetCondition(pcidevicecondition.ReadyType, pciDevice.Status.Conditions)
	return found && readyCondition.Status == metav1.ConditionTrue
}

func attachedHostDevicesByName(kvvmi *virtv1.VirtualMachineInstance) map[string]struct{} {
	attachedByName := make(map[string]struct{})
	if kvvmi == nil || kvvmi.Status.DeviceStatus == nil {
		return attachedByName
	}
	for _, hostDeviceStatus := range kvvmi.Status.DeviceStatus.HostDeviceStatuses {
		if hostDeviceStatus.Name == "" {
			continue
		}
		if hostDeviceStatus.Phase == virtv1.DeviceReady {
			attachedByName[hostDeviceStatus.Name] = struct{}{}
		}
	}
	return attachedByName
}
