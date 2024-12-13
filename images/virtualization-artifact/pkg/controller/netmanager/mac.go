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

package netmanager

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewMACManager() *MACManager {
	return &MACManager{}
}

type MACManager struct{}

func (m MACManager) IsBound(vmName string, vmmac *virtv2.VirtualMachineMACAddress) bool {
	if vmmac == nil {
		return false
	}

	if vmmac.Status.Phase != virtv2.VirtualMachineMACAddressPhaseBound && vmmac.Status.Phase != virtv2.VirtualMachineMACAddressPhaseAttached {
		return false
	}

	return vmmac.Status.VirtualMachine == vmName
}

func (m MACManager) CheckMACAddressAvailableForBinding(vmmac *virtv2.VirtualMachineMACAddress) error {
	if vmmac == nil {
		return errors.New("cannot to bind with empty MAC address")
	}

	return nil
}

func (m MACManager) CreateMACAddress(ctx context.Context, vm *virtv2.VirtualMachine, client client.Client, ifName, macAddress string) error {
	ownerRef := metav1.NewControllerRef(vm, vm.GroupVersionKind())
	vmmac := &virtv2.VirtualMachineMACAddress{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				annotations.LabelVirtualMachineUID: string(vm.GetUID()),
			},
			GenerateName:    GenerateName(vm),
			Namespace:       vm.Namespace,
			OwnerReferences: []metav1.OwnerReference{*ownerRef},
		},
		Spec: virtv2.VirtualMachineMACAddressSpec{
			InterfaceName: ifName,
		},
	}
	if macAddress != "" {
		vmmac.Spec.Address = macAddress
	}

	return client.Create(ctx, vmmac)
}
