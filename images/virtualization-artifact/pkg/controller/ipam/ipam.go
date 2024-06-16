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

func (m IPAM) IsBound(vmName string, claim *virtv2.VirtualMachineIPAddressClaim) bool {
	if claim == nil {
		return false
	}

	if claim.Status.Phase != virtv2.VirtualMachineIPAddressClaimPhaseBound {
		return false
	}

	return claim.Status.VirtualMachine == vmName
}

func (m IPAM) CheckClaimAvailableForBinding(vmName string, claim *virtv2.VirtualMachineIPAddressClaim) error {
	if claim == nil {
		return errors.New("cannot to bind with empty claim")
	}

	boundVMName := claim.Status.VirtualMachine
	if boundVMName == "" || boundVMName == vmName {
		return nil
	}

	return fmt.Errorf(
		"unable to bind the claim (%s) to the virtual machine (%s): the claim has already been linked to another one",
		claim.Name,
		vmName,
	)
}

func (m IPAM) CreateIPAddressClaim(ctx context.Context, vm *virtv2.VirtualMachine, client client.Client) error {
	return client.Create(ctx, &virtv2.VirtualMachineIPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				common.LabelImplicitIPAddressClaim: common.LabelImplicitIPAddressClaimValue,
			},
			Name:      vm.Name,
			Namespace: vm.Namespace,
		},
		Spec: virtv2.VirtualMachineIPAddressClaimSpec{
			ReclaimPolicy: virtv2.VirtualMachineIPAddressReclaimPolicyDelete,
		},
	})
}

func (m IPAM) DeleteIPAddressClaim(ctx context.Context, claim *virtv2.VirtualMachineIPAddressClaim, client client.Client) error {
	return client.Delete(ctx, claim)
}
