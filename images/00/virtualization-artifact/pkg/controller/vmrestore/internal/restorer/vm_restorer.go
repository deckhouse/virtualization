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

package restorer

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineOverrideValidator struct {
	vm     *virtv2.VirtualMachine
	client client.Client
}

func NewVirtualMachineOverrideValidator(vmTmpl *virtv2.VirtualMachine, client client.Client) *VirtualMachineOverrideValidator {
	return &VirtualMachineOverrideValidator{
		vm: &virtv2.VirtualMachine{
			TypeMeta: metav1.TypeMeta{
				Kind:       vmTmpl.Kind,
				APIVersion: vmTmpl.APIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        vmTmpl.Name,
				Namespace:   vmTmpl.Namespace,
				Annotations: vmTmpl.Annotations,
				Labels:      vmTmpl.Labels,
			},
			Spec: vmTmpl.Spec,
		},
		client: client,
	}
}

func (v *VirtualMachineOverrideValidator) Override(rules []virtv2.NameReplacement) {
	v.vm.Name = overrideName(v.vm.Kind, v.vm.Name, rules)
	v.vm.Spec.VirtualMachineIPAddress = overrideName(virtv2.VirtualMachineIPAddressKind, v.vm.Spec.VirtualMachineIPAddress, rules)

	for i := range v.vm.Spec.BlockDeviceRefs {
		if v.vm.Spec.BlockDeviceRefs[i].Kind != virtv2.DiskDevice {
			continue
		}

		v.vm.Spec.BlockDeviceRefs[i].Name = overrideName(virtv2.VirtualDiskKind, v.vm.Spec.BlockDeviceRefs[i].Name, rules)
	}
}

func (v *VirtualMachineOverrideValidator) Validate(ctx context.Context) error {
	vmKey := types.NamespacedName{Namespace: v.vm.Namespace, Name: v.vm.Name}
	existed, err := object.FetchObject(ctx, vmKey, v.client, &virtv2.VirtualMachine{})
	if err != nil {
		return err
	}

	if existed != nil {
		return fmt.Errorf("the virtual machine %q %w", vmKey.Name, ErrAlreadyExists)
	}

	return nil
}

func (v *VirtualMachineOverrideValidator) Object() client.Object {
	return v.vm
}
