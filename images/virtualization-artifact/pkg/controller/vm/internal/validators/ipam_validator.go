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

package validators

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type IPAMValidator struct {
	ipam   internal.IPAM
	client client.Client
}

func NewIPAMValidator(ipam internal.IPAM, client client.Client) *IPAMValidator {
	return &IPAMValidator{ipam: ipam, client: client}
}

func (v *IPAMValidator) ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	vmipName := vm.Spec.VirtualMachineIPAddress
	if vmipName == "" {
		vmipName = vm.Name
	}

	vmipKey := types.NamespacedName{Name: vmipName, Namespace: vm.Namespace}
	vmip, err := helper.FetchObject(ctx, vmipKey, v.client, &v1alpha2.VirtualMachineIPAddress{})
	if err != nil {
		return nil, fmt.Errorf("unable to get referenced VirtualMachineIPAddress %s: %w", vmipKey, err)
	}

	if vmip == nil {
		return nil, nil
	}

	// VM is created without ip address, but ip address resource is already exists.
	if vm.Spec.VirtualMachineIPAddress == "" {
		return nil, fmt.Errorf("VirtualMachineIPAddress with the name of the virtual machine"+
			" already exists: set spec.virtualMachineIPAddress field to %s to use IP %s", vmip.Name, vmip.Status.Address)
	}

	return nil, v.ipam.CheckIpAddressAvailableForBinding(vm.Name, vmip)
}

func (v *IPAMValidator) ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if oldVM.Spec.VirtualMachineIPAddress == newVM.Spec.VirtualMachineIPAddress {
		return nil, nil
	}

	if newVM.Spec.VirtualMachineIPAddress == "" {
		//return nil, fmt.Errorf("spec.virtualMachineIPAddress cannot be changed to an empty value once set")
		return nil, nil
	}

	vmipKey := types.NamespacedName{Name: newVM.Spec.VirtualMachineIPAddress, Namespace: newVM.Namespace}
	vmip, err := helper.FetchObject(ctx, vmipKey, v.client, &v1alpha2.VirtualMachineIPAddress{})
	if err != nil {
		return nil, fmt.Errorf("unable to get VirtualMachineIPAddress %s: %w", vmipKey, err)
	}

	if vmip == nil {
		return nil, nil
	}

	return nil, v.ipam.CheckIpAddressAvailableForBinding(newVM.Name, vmip)
}
