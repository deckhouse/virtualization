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
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineIPAddressOverrideValidator struct {
	vmip   *virtv2.VirtualMachineIPAddress
	client client.Client
}

func NewVirtualMachineIPAddressOverrideValidator(vmipTmpl *virtv2.VirtualMachineIPAddress, client client.Client) *VirtualMachineIPAddressOverrideValidator {
	return &VirtualMachineIPAddressOverrideValidator{
		vmip: &virtv2.VirtualMachineIPAddress{
			TypeMeta: metav1.TypeMeta{
				Kind:       vmipTmpl.Kind,
				APIVersion: vmipTmpl.APIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        vmipTmpl.Name,
				Namespace:   vmipTmpl.Namespace,
				Annotations: vmipTmpl.Annotations,
				Labels:      vmipTmpl.Labels,
			},
			Spec: vmipTmpl.Spec,
		},
		client: client,
	}
}

func (v *VirtualMachineIPAddressOverrideValidator) Override(rules []virtv2.NameReplacement) {
	v.vmip.Name = overrideName(v.vmip.Kind, v.vmip.Name, rules)
}

func (v *VirtualMachineIPAddressOverrideValidator) Validate(ctx context.Context) error {
	vmipKey := types.NamespacedName{Namespace: v.vmip.Namespace, Name: v.vmip.Name}
	existed, err := object.FetchObject(ctx, vmipKey, v.client, &virtv2.VirtualMachineIPAddress{})
	if err != nil {
		return err
	}

	if existed == nil {
		if v.vmip.Spec.StaticIP == "" {
			return nil
		}

		var vmips virtv2.VirtualMachineIPAddressList
		err = v.client.List(ctx, &vmips, &client.ListOptions{
			Namespace:     v.vmip.Namespace,
			FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVMIPByAddress, v.vmip.Spec.StaticIP),
		})
		if err != nil {
			return err
		}

		if len(vmips.Items) > 0 {
			return fmt.Errorf(
				"the set address %q is %w by the different virtual machine ip address %q and cannot be used for the restored virtual machine",
				v.vmip.Spec.StaticIP, ErrAlreadyInUse, vmips.Items[0].Name,
			)
		}

		return nil
	}

	if existed.Status.Phase == virtv2.VirtualMachineIPAddressPhaseAttached || existed.Status.VirtualMachine != "" {
		return fmt.Errorf("the virtual machine ip address %q is %w and cannot be used for the restored virtual machine", vmipKey.Name, ErrAlreadyInUse)
	}

	return nil
}

func (v *VirtualMachineIPAddressOverrideValidator) Object() client.Object {
	return v.vmip
}
