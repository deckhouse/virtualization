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
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/kubeapi"
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
	if kubeapi.HasDRAPartitionableDevices() {
		return v.validateAvailableUSBIPPortsWithPartitionableDevices(ctx, vm, oldUSBDevices)
	}
	return v.validateAvailableUSBIPPortsDefault(ctx, vm, oldUSBDevices)
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

		isHs, isSS := resolveSpeed(usbDevice.Status.Attributes.Speed)
		switch {
		case isHs:
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

	node, totalPorts, err := v.getNodeTotalPorts(ctx, vm.Status.Node)
	if err != nil {
		return admission.Warnings{}, err
	}

	totalPortsPerHub := totalPorts / 2

	if len(hsUSBFromOtherNodes) > 0 {
		if err = validateUsedPortsByAnnotation(node, annotations.AnnUSBIPHighSpeedHubUsedPorts, hsUSBFromOtherNodes, totalPortsPerHub); err != nil {
			return admission.Warnings{}, err
		}
	}

	if len(ssUSBFromOtherNodes) > 0 {
		if err = validateUsedPortsByAnnotation(node, annotations.AnnUSBIPSuperSpeedHubUsedPorts, ssUSBFromOtherNodes, totalPortsPerHub); err != nil {
			return admission.Warnings{}, err
		}
	}

	return admission.Warnings{}, nil
}

func (v *USBDevicesValidator) getNodeTotalPorts(ctx context.Context, nodeName string) (*corev1.Node, int, error) {
	node := &corev1.Node{}
	err := v.client.Get(ctx, client.ObjectKey{Name: nodeName}, node)
	if err != nil {
		return nil, -1, fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	totalPorts, exists := node.Annotations[annotations.AnnUSBIPTotalPorts]
	if !exists {
		return nil, -1, fmt.Errorf("node %s does not have %s annotation", nodeName, annotations.AnnUSBIPTotalPorts)
	}
	totalPortsInt, err := strconv.Atoi(totalPorts)
	if err != nil {
		return nil, -1, fmt.Errorf("failed to parse %s annotation: %w", annotations.AnnUSBIPTotalPorts, err)
	}

	return node, totalPortsInt, nil
}

func validateUsedPortsByAnnotation(node *corev1.Node, anno string, usbFromOtherNodes []string, totalPortsPerHub int) error {
	usedPorts, exists := node.Annotations[anno]
	if !exists {
		return fmt.Errorf("node %s does not have %s annotation", node.Name, anno)
	}
	usedPortsInt, err := strconv.Atoi(usedPorts)
	if err != nil {
		return fmt.Errorf("failed to parse %s annotation: %w", anno, err)
	}

	wantedPorts := usedPortsInt + len(usbFromOtherNodes)
	if wantedPorts > totalPortsPerHub {
		return fmt.Errorf("node %s not available ports for sharing USB devices %s. total: %d, used: %d, wanted: %d", node.Name, strings.Join(usbFromOtherNodes, ", "), totalPortsPerHub, usedPortsInt, wantedPorts)
	}

	return nil
}

// https://mjmwired.net/kernel/Documentation/ABI/testing/sysfs-bus-usb#502
func resolveSpeed(speed int) (isHs, isSS bool) {
	return speed == 480, speed >= 5000
}

func (v *USBDevicesValidator) validateAvailableUSBIPPortsDefault(ctx context.Context, vm *v1alpha2.VirtualMachine, oldUSBDevices map[string]struct{}) (admission.Warnings, error) {
	if vm.Status.Node == "" {
		return admission.Warnings{}, nil
	}
	if vm.Spec.USBDevices == nil {
		return admission.Warnings{}, nil
	}

	var usbFromOtherNodes []string

	for _, ref := range vm.Spec.USBDevices {
		if _, exists := oldUSBDevices[ref.Name]; exists {
			continue
		}

		usbDevice := &v1alpha2.USBDevice{}
		err := v.client.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: vm.Namespace}, usbDevice)
		if err != nil {
			return admission.Warnings{}, fmt.Errorf("failed to get USB device %s: %w", ref.Name, err)
		}

		if usbDevice.Status.NodeName != vm.Status.Node {
			usbFromOtherNodes = append(usbFromOtherNodes, ref.Name)
		}
	}

	if len(usbFromOtherNodes) == 0 {
		return admission.Warnings{}, nil
	}

	node, totalPorts, err := v.getNodeTotalPorts(ctx, vm.Status.Node)
	if err != nil {
		return admission.Warnings{}, err
	}

	// total for 2 usb hubs (2.0 and 3.0)
	totalPorts /= 2

	return admission.Warnings{}, validateUsedPortsByAnnotation(node, annotations.AnnUSBIPUsedPorts, usbFromOtherNodes, totalPorts)
}
