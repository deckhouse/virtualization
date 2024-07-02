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

package ipam

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const AnnoIPAddressCNIRequest = "cni.cilium.io/ipAddress"

func New() *IPAM {
	return &IPAM{}
}

type IPAM struct{}

func (m IPAM) IsBound(vmName string, vmip *virtv2.VirtualMachineIPAddress) bool {
	if vmip == nil {
		return false
	}

	if vmip.Status.Phase != virtv2.VirtualMachineIPAddressPhaseBound {
		return false
	}

	return vmip.Status.VirtualMachine == vmName
}

func (m IPAM) CheckIpAddressAvailableForBinding(vmName string, vmip *virtv2.VirtualMachineIPAddress) error {
	if vmip == nil {
		return errors.New("cannot to bind with empty ip address")
	}

	boundVMName := vmip.Status.VirtualMachine
	if boundVMName == "" || boundVMName == vmName {
		return nil
	}

	return fmt.Errorf(
		"unable to bind the ip address (%s) to the virtual machine (%s): the ip address has already been linked to another one",
		vmip.Name,
		vmName,
	)
}

func (m IPAM) CreateIPAddress(ctx context.Context, vm *virtv2.VirtualMachine, client client.Client) error {
	return client.Create(ctx, &virtv2.VirtualMachineIPAddress{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				common.LabelImplicitIPAddress: common.LabelImplicitIPAddressValue,
			},
			Name:      vm.Name,
			Namespace: vm.Namespace,
		},
		Spec: virtv2.VirtualMachineIPAddressSpec{
			ReclaimPolicy: virtv2.VirtualMachineIPAddressReclaimPolicyDelete,
		},
	})
}

func (m IPAM) DeleteIPAddress(ctx context.Context, vmip *virtv2.VirtualMachineIPAddress, client client.Client) error {
	return client.Delete(ctx, vmip)
}
