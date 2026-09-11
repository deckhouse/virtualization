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

package validators

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type PCIDevicesValidator struct {
	client      client.Client
	featureGate featuregate.FeatureGate
}

func NewPCIDevicesValidator(client client.Client, featureGate featuregate.FeatureGate) *PCIDevicesValidator {
	return &PCIDevicesValidator{client: client, featureGate: featureGate}
}

func (v *PCIDevicesValidator) ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.validate(ctx, vm)
}

func (v *PCIDevicesValidator) ValidateUpdate(ctx context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.validate(ctx, newVM)
}

func (v *PCIDevicesValidator) validate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if len(vm.Spec.PCIDevices) == 0 {
		return nil, nil
	}

	if !v.featureGate.Enabled(featuregates.PCI) {
		return nil, fmt.Errorf("PCI device attachment requires Kubernetes version 1.34 or newer and enabled DRA feature gates")
	}

	if err := v.validatePCIDevicesUnique(ctx, vm); err != nil {
		return nil, err
	}

	return nil, v.validatePCIDevicesOnSingleNode(ctx, vm)
}

// validatePCIDevicesUnique checks for duplicates in the list and that each
// PCI device is not used by another VM in the namespace.
func (v *PCIDevicesValidator) validatePCIDevicesUnique(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	seen := make(map[string]struct{})
	for _, ref := range vm.Spec.PCIDevices {
		if ref.Name == "" {
			continue
		}
		if _, exists := seen[ref.Name]; exists {
			return fmt.Errorf("duplicate PCI device %s in spec.pciDevices", ref.Name)
		}
		seen[ref.Name] = struct{}{}

		var vmList v1alpha2.VirtualMachineList
		if err := v.client.List(ctx, &vmList, client.InNamespace(vm.Namespace), client.MatchingFields{indexer.IndexFieldVMByPCIDevice: ref.Name}); err != nil {
			return fmt.Errorf("failed to list VMs using PCI device %s: %w", ref.Name, err)
		}

		for i := range vmList.Items {
			otherVM := &vmList.Items[i]
			if otherVM.Name == vm.Name {
				continue
			}
			return fmt.Errorf("PCI device %s is already used by VirtualMachine %s/%s", ref.Name, otherVM.Namespace, otherVM.Name)
		}
	}

	return nil
}

// validatePCIDevicesOnSingleNode rejects a set of PCI devices that resides on
// more than one node: a PCI device is usable only by a virtual machine running
// on its node, so such a set could never be satisfied.
func (v *PCIDevicesValidator) validatePCIDevicesOnSingleNode(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	nodeName := ""
	nodeOwner := ""
	for _, ref := range vm.Spec.PCIDevices {
		if ref.Name == "" {
			continue
		}

		pciDevice := &v1alpha2.PCIDevice{}
		err := v.client.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: vm.Namespace}, pciDevice)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("failed to get PCI device %s: %w", ref.Name, err)
		}

		if pciDevice.Status.NodeName == "" {
			continue
		}
		if nodeName == "" {
			nodeName = pciDevice.Status.NodeName
			nodeOwner = ref.Name
			continue
		}
		if pciDevice.Status.NodeName != nodeName {
			return fmt.Errorf("PCI devices %s and %s reside on different nodes (%s and %s); all PCI devices of a virtual machine must reside on the same node",
				nodeOwner, ref.Name, nodeName, pciDevice.Status.NodeName)
		}
	}

	return nil
}
