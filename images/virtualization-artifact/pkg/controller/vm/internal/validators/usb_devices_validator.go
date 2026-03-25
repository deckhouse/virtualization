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

package validators

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/usb"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type USBDevicesValidator struct {
	client      client.Client
	featureGate featuregate.FeatureGate
}

func NewUSBDevicesValidator(client client.Client, featureGate featuregate.FeatureGate) *USBDevicesValidator {
	return &USBDevicesValidator{client: client, featureGate: featureGate}
}

func (v *USBDevicesValidator) ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if err := v.validateUSBFeature(vm); err != nil {
		return nil, err
	}

	return v.validateUSBDevicesUnique(ctx, vm, nil)
}

func (v *USBDevicesValidator) ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if err := v.validateUSBFeature(newVM); err != nil {
		return nil, err
	}

	if equality.Semantic.DeepEqual(oldVM.Spec.USBDevices, newVM.Spec.USBDevices) {
		return nil, nil
	}

	oldUsbDevices := getUSBDeviceNames(oldVM.Spec.USBDevices)

	var allWarnings admission.Warnings

	if warnings, err := v.validateUSBDevicesUnique(ctx, newVM, oldUsbDevices); err != nil {
		allWarnings = append(allWarnings, warnings...)
		return allWarnings, err
	} else {
		allWarnings = append(allWarnings, warnings...)
	}

	if warnings, err := v.validateAvailableUSBIPPorts(ctx, newVM, oldUsbDevices); err != nil {
		allWarnings = append(allWarnings, warnings...)
		return allWarnings, err
	} else {
		allWarnings = append(allWarnings, warnings...)
	}

	return allWarnings, nil
}

// validateUSBDevicesUnique checks that each USB device is not used by another VM.
// currentVMName is empty for Create (no VM to exclude), or VM name for Update (exclude current VM from conflict check).
func (v *USBDevicesValidator) validateUSBDevicesUnique(ctx context.Context, vm *v1alpha2.VirtualMachine, oldUSBDevices map[string]struct{}) (admission.Warnings, error) {
	if len(vm.Spec.USBDevices) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{})
	for _, ref := range vm.Spec.USBDevices {
		if ref.Name == "" {
			continue
		}
		if _, exists := seen[ref.Name]; exists {
			return nil, fmt.Errorf("duplicate USB device %s in spec.usbDevices", ref.Name)
		}
		seen[ref.Name] = struct{}{}

		if _, exists := oldUSBDevices[ref.Name]; exists {
			continue
		}

		var vmList v1alpha2.VirtualMachineList
		if err := v.client.List(ctx, &vmList, client.InNamespace(vm.Namespace), client.MatchingFields{indexer.IndexFieldVMByUSBDevice: ref.Name}); err != nil {
			return nil, fmt.Errorf("failed to list VMs using USB device %s: %w", ref.Name, err)
		}

		for i := range vmList.Items {
			otherVM := &vmList.Items[i]
			if otherVM.Name == vm.Name {
				continue
			}
			return nil, fmt.Errorf("USB device %s is already used by VirtualMachine %s/%s", ref.Name, otherVM.Namespace, otherVM.Name)
		}
	}

	return nil, nil
}

func (v *USBDevicesValidator) validateUSBFeature(vm *v1alpha2.VirtualMachine) error {
	if len(vm.Spec.USBDevices) == 0 {
		return nil
	}

	if v.featureGate.Enabled(featuregates.USB) {
		return nil
	}

	return fmt.Errorf("USB device attachment requires Kubernetes version 1.34 or newer and enabled DRA feature gates")
}

func getUSBDeviceNames(refs []v1alpha2.USBDeviceSpecRef) map[string]struct{} {
	names := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if ref.Name == "" {
			continue
		}
		names[ref.Name] = struct{}{}
	}

	return names
}

func (v *USBDevicesValidator) validateAvailableUSBIPPorts(ctx context.Context, vm *v1alpha2.VirtualMachine, oldUSBDevices map[string]struct{}) (admission.Warnings, error) {
	return v.validateAvailableUSBIPPortsWithPartitionableDevices(ctx, vm, oldUSBDevices)
}

func (v *USBDevicesValidator) validateAvailableUSBIPPortsWithPartitionableDevices(ctx context.Context, vm *v1alpha2.VirtualMachine, oldUSBDevices map[string]struct{}) (admission.Warnings, error) {
	if vm.Status.Node == "" {
		return admission.Warnings{}, nil
	}
	if vm.Spec.USBDevices == nil {
		return admission.Warnings{}, nil
	}

	var hsUSBFromOtherNodes []string
	var ssUSBFromOtherNodes []string

	for _, ref := range vm.Spec.USBDevices {
		if _, exists := oldUSBDevices[ref.Name]; exists {
			continue
		}

		usbDevice := &v1alpha2.USBDevice{}
		err := v.client.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: vm.Namespace}, usbDevice)
		if err != nil {
			return admission.Warnings{}, fmt.Errorf("failed to get USB device %s: %w", ref.Name, err)
		}

		if usbDevice.Status.NodeName == vm.Status.Node {
			continue
		}

		isHS, isSS := usb.ResolveSpeed(usbDevice.Status.Attributes.Speed)
		switch {
		case isHS:
			hsUSBFromOtherNodes = append(hsUSBFromOtherNodes, ref.Name)
		case isSS:
			ssUSBFromOtherNodes = append(ssUSBFromOtherNodes, ref.Name)
		default:
			return admission.Warnings{}, fmt.Errorf("USB device %s has unsupported speed %d", ref.Name, usbDevice.Status.Attributes.Speed)
		}
	}

	if len(hsUSBFromOtherNodes) == 0 && len(ssUSBFromOtherNodes) == 0 {
		return admission.Warnings{}, nil
	}

	node := &corev1.Node{}
	err := v.client.Get(ctx, client.ObjectKey{Name: vm.Status.Node}, node)
	if err != nil {
		return admission.Warnings{}, err
	}

	if len(hsUSBFromOtherNodes) > 0 {
		hasFree, err := usb.CheckFreePortForRequest(node.Annotations, 480, len(hsUSBFromOtherNodes))
		if err != nil {
			return admission.Warnings{}, err
		}
		if !hasFree {
			return admission.Warnings{}, fmt.Errorf("node %s has no available ports for sharing USB devices %v", vm.Status.Node, hsUSBFromOtherNodes)
		}
	}

	if len(ssUSBFromOtherNodes) > 0 {
		hasFree, err := usb.CheckFreePortForRequest(node.Annotations, 5000, len(ssUSBFromOtherNodes))
		if err != nil {
			return admission.Warnings{}, err
		}
		if !hasFree {
			return admission.Warnings{}, fmt.Errorf("node %s has no available ports for sharing USB devices %v", vm.Status.Node, ssUSBFromOtherNodes)
		}
	}

	return admission.Warnings{}, nil
}
