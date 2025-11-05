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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

const AnnoIPAddressCNIRequest = "cni.cilium.io/ipAddress"

func NewIPAM() *IPAM {
	return &IPAM{}
}

type IPAM struct{}

func (m IPAM) IsBound(vmName string, vmip *v1alpha2.VirtualMachineIPAddress) bool {
	if vmip == nil {
		return false
	}

	boundCondition, _ := conditions.GetCondition(vmipcondition.BoundType, vmip.Status.Conditions)
	if boundCondition.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(boundCondition, vmip) {
		return false
	}

	// TODO: temporary solution to test performance.
	// attachedCondition, _ := conditions.GetCondition(vmipcondition.AttachedType, vmip.Status.Conditions)
	// if attachedCondition.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(attachedCondition, vmip) {
	// 	return false
	// }

	// return vmip.Status.VirtualMachine == vmName
	return true
}

func (m IPAM) CheckIPAddressAvailableForBinding(vmName string, vmip *v1alpha2.VirtualMachineIPAddress) error {
	if vmip == nil {
		return errors.New("cannot to bind with empty ip address")
	}

	return nil
}

func (m IPAM) CreateIPAddress(ctx context.Context, vm *v1alpha2.VirtualMachine, client client.Client) error {
	ownerRef := metav1.NewControllerRef(vm, vm.GroupVersionKind())
	return client.Create(ctx, &v1alpha2.VirtualMachineIPAddress{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				annotations.LabelVirtualMachineUID: string(vm.GetUID()),
			},
			GenerateName:    GenerateName(vm),
			Namespace:       vm.Namespace,
			OwnerReferences: []metav1.OwnerReference{*ownerRef},
		},
		Spec: v1alpha2.VirtualMachineIPAddressSpec{
			Type: v1alpha2.VirtualMachineIPAddressTypeAuto,
		},
	})
}

const generateNameSuffix = "-"

func GenerateName(vm *v1alpha2.VirtualMachine) string {
	if vm == nil {
		return ""
	}
	return vm.GetName() + generateNameSuffix
}

func GetVirtualMachineName(vmip *v1alpha2.VirtualMachineIPAddress) string {
	if vmip == nil {
		return ""
	}
	if gn := vmip.GenerateName; gn != "" {
		return strings.TrimSuffix(vmip.GenerateName, generateNameSuffix)
	}

	name := vmip.GetName()
	for _, ow := range vmip.GetOwnerReferences() {
		if ow.Kind == v1alpha2.VirtualMachineKind {
			name = ow.Name
			break
		}
	}
	return name
}
