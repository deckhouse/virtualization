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
	"strconv"
	"strings"

	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/usbdevicecondition"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

// VirtClient is an interface for accessing VirtualMachine resources with subresource operations.
type VirtClient interface {
	VirtualMachines(namespace string) virtualizationv1alpha2.VirtualMachineInterface
}

type usbDeviceHandlerBase struct {
	client     client.Client
	virtClient VirtClient
}

func (h *usbDeviceHandlerBase) getResourceClaimTemplateName(usbDeviceName string) string {
	return usbDeviceName + "-template"
}

func (h *usbDeviceHandlerBase) getResourceClaimRequestName(usbDeviceName string) string {
	return "req-" + usbDeviceName
}

func (h *usbDeviceHandlerBase) getResourceClaimTemplate(
	ctx context.Context,
	namespace string,
	templateName string,
) (*resourcev1.ResourceClaimTemplate, error) {
	template := &resourcev1.ResourceClaimTemplate{}
	key := types.NamespacedName{
		Name:      templateName,
		Namespace: namespace,
	}
	if err := h.client.Get(ctx, key, template); err != nil {
		return nil, fmt.Errorf("failed to get ResourceClaimTemplate: %w", err)
	}
	return template, nil
}

func (h *usbDeviceHandlerBase) isUSBDeviceReady(usbDevice *v1alpha2.USBDevice) bool {
	if usbDevice.Status.Attributes.VendorID == "" || usbDevice.Status.Attributes.ProductID == "" {
		return false
	}
	if usbDevice.Status.NodeName == "" {
		return false
	}
	readyCondition, found := conditions.GetCondition(usbdevicecondition.ReadyType, usbDevice.Status.Conditions)
	return found && readyCondition.Status == metav1.ConditionTrue
}

func (h *usbDeviceHandlerBase) hostDeviceReadyByName(kvvmi *virtv1.VirtualMachineInstance) map[string]bool {
	hostDeviceReadyByName := make(map[string]bool)
	if kvvmi == nil || kvvmi.Status.DeviceStatus == nil {
		return hostDeviceReadyByName
	}

	for _, hostDeviceStatus := range kvvmi.Status.DeviceStatus.HostDeviceStatuses {
		if hostDeviceStatus.Name == "" {
			continue
		}

		hostDeviceReadyByName[hostDeviceStatus.Name] = hostDeviceReadyByName[hostDeviceStatus.Name] || hostDeviceStatus.Phase == virtv1.DeviceReady
	}

	return hostDeviceReadyByName
}

func (h *usbDeviceHandlerBase) attachUSBDevice(
	ctx context.Context,
	vm *v1alpha2.VirtualMachine,
	usbDeviceName string,
	templateName string,
	requestName string,
) error {
	opts := subv1alpha2.VirtualMachineAddResourceClaim{
		Name:                      usbDeviceName,
		ResourceClaimTemplateName: templateName,
		RequestName:               requestName,
	}
	return h.virtClient.VirtualMachines(vm.Namespace).AddResourceClaim(ctx, vm.Name, opts)
}

func (h *usbDeviceHandlerBase) detachUSBDevice(
	ctx context.Context,
	vm *v1alpha2.VirtualMachine,
	usbDeviceName string,
) error {
	opts := subv1alpha2.VirtualMachineRemoveResourceClaim{
		Name: usbDeviceName,
	}
	return h.virtClient.VirtualMachines(vm.Namespace).RemoveResourceClaim(ctx, vm.Name, opts)
}

func (h *usbDeviceHandlerBase) getUSBAddressFromKVVMI(deviceName string, kvvmi *virtv1.VirtualMachineInstance) *v1alpha2.USBAddress {
	if kvvmi == nil || kvvmi.Status.DeviceStatus == nil {
		return nil
	}
	for _, st := range kvvmi.Status.DeviceStatus.HostDeviceStatuses {
		if st.Name != deviceName {
			continue
		}

		if st.Address == "" {
			continue
		}

		return parseUSBAddress(st.Address)
	}
	return nil
}

func parseUSBAddress(address string) *v1alpha2.USBAddress {
	parts := strings.Split(address, ":")
	if len(parts) != 2 {
		return nil
	}

	busPart := strings.TrimSpace(parts[0])
	portPart := strings.TrimSpace(parts[1])
	if busPart == "" || portPart == "" {
		return nil
	}

	bus, err := strconv.Atoi(busPart)
	if err != nil {
		return nil
	}

	port, err := strconv.Atoi(portPart)
	if err != nil {
		return nil
	}

	return &v1alpha2.USBAddress{
		Bus:  bus,
		Port: port,
	}
}

func (h *usbDeviceHandlerBase) hasPendingMigrationOp(ctx context.Context, s state.VirtualMachineState) (bool, error) {
	vmops, err := s.VMOPs(ctx)
	if err != nil {
		return false, err
	}

	for _, vmop := range vmops {
		if (vmop.Spec.Type == v1alpha2.VMOPTypeEvict || vmop.Spec.Type == v1alpha2.VMOPTypeMigrate) && vmop.Status.Phase == v1alpha2.VMOPPhasePending {
			return true, nil
		}
	}
	return false, nil
}
