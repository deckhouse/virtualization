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
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type IPAMValidator struct {
	client              client.Client
	virtualMachineCIDRs []string
}

func NewIPAMValidator(client client.Client, virtualMachineCIDRs []string) *IPAMValidator {
	return &IPAMValidator{client: client, virtualMachineCIDRs: virtualMachineCIDRs}
}

// The controller drops this warning by its prefix: see vmipDeprecationPrefixes in
// cmd/virtualization-controller.
const vmipDeprecationWarning = "spec.virtualMachineIPAddressName is deprecated: the address of the Main network is managed by the IPAddress resources of the sdn module; use spec.networks[Main].ipAddressName instead"

// deprecationWarnings warns only where the sdn module serves the Main network: elsewhere the advice cannot be followed.
func (v *IPAMValidator) deprecationWarnings(ctx context.Context, vmipName string) admission.Warnings {
	if vmipName == "" {
		return nil
	}
	mainIPAMAvailable, err := network.MainIPAMAvailable(ctx, v.client)
	if err != nil {
		logger.FromContext(ctx).Error("Failed to check whether the sdn module serves the Main network", logger.SlogErr(err))
		return nil
	}
	if !mainIPAMAvailable {
		return nil
	}
	return admission.Warnings{vmipDeprecationWarning}
}

func (v *IPAMValidator) ValidateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if err := v.validateVMIPUsage(vm); err != nil {
		return nil, err
	}

	if err := v.validateMainIPAddressName(ctx, vm); err != nil {
		return nil, err
	}

	warnings := v.deprecationWarnings(ctx, vm.Spec.VirtualMachineIPAddress)

	vmipName := vm.Spec.VirtualMachineIPAddress
	if vmipName == "" {
		vmipName = vm.Name
	}

	vmipKey := types.NamespacedName{Name: vmipName, Namespace: vm.Namespace}
	vmip, err := object.FetchObject(ctx, vmipKey, v.client, &v1alpha2.VirtualMachineIPAddress{})
	if err != nil {
		return nil, fmt.Errorf("unable to get referenced VirtualMachineIPAddress %s: %w", vmipKey, err)
	}

	if vmip == nil {
		return warnings, nil
	}

	// VM is created without ip address, but ip address resource is already exists.
	if vm.Spec.VirtualMachineIPAddress == "" {
		// Where the sdn module serves the Main network the machine gets an IPAddress
		// and never binds the vmip, so the vmip is no obstacle.
		mainIPAMAvailable, err := network.MainIPAMAvailable(ctx, v.client)
		if err != nil {
			return warnings, fmt.Errorf("unable to check whether the sdn module serves the Main network: %w", err)
		}
		if !mainIPAMAvailable {
			return warnings, fmt.Errorf("VirtualMachineIPAddress with the name of the virtual machine"+
				" already exists: set spec.virtualMachineIPAddress field to %s to use IP %s", vmip.Name, vmip.Status.Address)
		}
	}

	return warnings, nil
}

func (v *IPAMValidator) ValidateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	ipChanged := oldVM.Spec.VirtualMachineIPAddress != newVM.Spec.VirtualMachineIPAddress
	networksChanged := !equality.Semantic.DeepEqual(oldVM.Spec.Networks, newVM.Spec.Networks)

	// Skip vmip validation if related fields are not changed.
	// Invalid network spec should not prevent metadata or status fields update.
	var warnings admission.Warnings
	if ipChanged {
		warnings = v.deprecationWarnings(ctx, newVM.Spec.VirtualMachineIPAddress)
	}

	if ipChanged || networksChanged {
		if err := v.validateVMIPUsage(newVM); err != nil {
			return warnings, err
		}
	}
	// Without the pool a named IPAddress stays in effect for a machine whose template
	// has the Main interface spec, so only a new name is checked.
	if mainIPAddressName(oldVM) != mainIPAddressName(newVM) {
		if err := v.validateMainIPAddressName(ctx, newVM); err != nil {
			return warnings, err
		}
	}

	return warnings, nil
}

func mainIPAddressName(vm *v1alpha2.VirtualMachine) string {
	if mainSpec := network.GetMainNetworkSpec(vm.Spec.Networks); mainSpec != nil {
		return mainSpec.IPAddressName
	}
	return ""
}

func (v *IPAMValidator) validateVMIPUsage(vm *v1alpha2.VirtualMachine) error {
	if vm.Spec.VirtualMachineIPAddress == "" {
		return nil
	}

	if mainSpec := network.GetMainNetworkSpec(vm.Spec.Networks); mainSpec != nil && mainSpec.IPAddressName != "" {
		return fmt.Errorf("spec.virtualMachineIPAddressName cannot be combined with spec.networks[Main].ipAddressName: use only one way to set the Main network address")
	}

	if len(v.virtualMachineCIDRs) == 0 {
		return fmt.Errorf("spec.virtualMachineIPAddressName cannot be set when ModuleConfig/virtualization spec.settings.virtualMachineCIDRs is not configured")
	}

	if len(vm.Spec.Networks) > 0 && !network.HasMainNetworkSpec(vm.Spec.Networks) {
		return fmt.Errorf("spec.virtualMachineIPAddressName cannot be set without Main network type in spec.networks")
	}

	return nil
}

func (v *IPAMValidator) validateMainIPAddressName(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	mainSpec := network.GetMainNetworkSpec(vm.Spec.Networks)
	if mainSpec == nil || mainSpec.IPAddressName == "" {
		return nil
	}

	mainIPAMAvailable, err := network.MainIPAMAvailable(ctx, v.client)
	if err != nil {
		return fmt.Errorf("unable to check whether the sdn module serves the Main network: %w", err)
	}
	if !mainIPAMAvailable {
		return errors.New("spec.networks[Main].ipAddressName cannot be used: the Main network has no address pool in this cluster, so the virtual machine would never get the address. Remove the field to get an address automatically, or ask the cluster administrator to set up the address pool of the Main network")
	}

	// A missing IPAddress is not rejected: the user may create it after the machine.
	ipa := &sdnapi.IPAddress{}
	err = v.client.Get(ctx, types.NamespacedName{Name: mainSpec.IPAddressName, Namespace: vm.Namespace}, ipa)
	switch {
	case k8serrors.IsNotFound(err):
		return nil
	case err != nil:
		return fmt.Errorf("unable to get the IPAddress %q of the Main network: %w", mainSpec.IPAddressName, err)
	case !network.IsMainNetworkIPAddress(ipa):
		return fmt.Errorf("the IPAddress %q referenced by spec.networks[Main].ipAddressName belongs to another network: the Main network address must reference an IPAddress of the ClusterNetwork %q", mainSpec.IPAddressName, network.MainClusterNetworkName)
	}
	return nil
}
