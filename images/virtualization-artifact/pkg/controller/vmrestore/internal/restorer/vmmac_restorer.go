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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineMACAddressOverrideValidator struct {
	vmmac        *virtv2.VirtualMachineMACAddress
	client       client.Client
	vmRestoreUID string
}

func NewVirtualMachineMACAddressOverrideValidator(vmmacTmpl *virtv2.VirtualMachineMACAddress, client client.Client, vmRestoreUID string) *VirtualMachineMACAddressOverrideValidator {
	if vmmacTmpl.Annotations != nil {
		vmmacTmpl.Annotations[annotations.AnnVMRestore] = vmRestoreUID
	} else {
		vmmacTmpl.Annotations = make(map[string]string)
		vmmacTmpl.Annotations[annotations.AnnVMRestore] = vmRestoreUID
	}
	return &VirtualMachineMACAddressOverrideValidator{
		vmmac: &virtv2.VirtualMachineMACAddress{
			TypeMeta: metav1.TypeMeta{
				Kind:       vmmacTmpl.Kind,
				APIVersion: vmmacTmpl.APIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        vmmacTmpl.Name,
				Namespace:   vmmacTmpl.Namespace,
				Annotations: vmmacTmpl.Annotations,
				Labels:      vmmacTmpl.Labels,
			},
			Spec:   vmmacTmpl.Spec,
			Status: vmmacTmpl.Status,
		},
		client:       client,
		vmRestoreUID: vmRestoreUID,
	}
}

func (v *VirtualMachineMACAddressOverrideValidator) Override(rules []virtv2.NameReplacement) {
	v.vmmac.Name = overrideName(v.vmmac.Kind, v.vmmac.Name, rules)
}

func (v *VirtualMachineMACAddressOverrideValidator) Validate(ctx context.Context) error {
	vmmacKey := types.NamespacedName{Namespace: v.vmmac.Namespace, Name: v.vmmac.Name}
	existed, err := object.FetchObject(ctx, vmmacKey, v.client, &virtv2.VirtualMachineMACAddress{})
	if err != nil {
		return err
	}

	if existed == nil {
		if v.vmmac.Spec.Address == "" {
			return nil
		}

		var vmmacs virtv2.VirtualMachineMACAddressList
		err = v.client.List(ctx, &vmmacs, &client.ListOptions{
			Namespace:     v.vmmac.Namespace,
			FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVMMACByAddress, v.vmmac.Spec.Address),
		})
		if err != nil {
			return err
		}

		if len(vmmacs.Items) > 0 {
			return fmt.Errorf(
				"the set address %q is %w by the different virtual machine mac address %q and cannot be used for the restored virtual machine",
				v.vmmac.Spec.Address, ErrAlreadyInUse, vmmacs.Items[0].Name,
			)
		}

		return nil
	}

	if existed.Status.Phase == virtv2.VirtualMachineMACAddressPhaseAttached || existed.Status.VirtualMachine != "" {
		return fmt.Errorf("the virtual machine mac address %q is %w and cannot be used for the restored virtual machine", vmmacKey.Name, ErrAlreadyInUse)
	}

	return nil
}

func (v *VirtualMachineMACAddressOverrideValidator) ValidateWithForce(ctx context.Context) error {
	vmmacKey := types.NamespacedName{Namespace: v.vmmac.Namespace, Name: v.vmmac.Name}
	existed, err := object.FetchObject(ctx, vmmacKey, v.client, &virtv2.VirtualMachineMACAddress{})
	if err != nil {
		return err
	}

	vmName := v.vmmac.Status.VirtualMachine

	if existed == nil {
		if v.vmmac.Spec.Address == "" {
			return nil
		}

		var vmmacs virtv2.VirtualMachineMACAddressList
		err = v.client.List(ctx, &vmmacs, &client.ListOptions{
			Namespace:     v.vmmac.Namespace,
			FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVMMACByAddress, v.vmmac.Spec.Address),
		})
		if err != nil {
			return err
		}

		if len(vmmacs.Items) > 0 {
			return fmt.Errorf(
				"the set address %q is %w by the different virtual machine mac address %q and cannot be used for the restored virtual machine",
				v.vmmac.Spec.Address, ErrAlreadyInUse, vmmacs.Items[0].Name,
			)
		}

		return nil
	}

	if existed.Status.Phase == virtv2.VirtualMachineMACAddressPhaseAttached && existed.Status.VirtualMachine == vmName {
		return ErrAlreadyExists
	}

	if existed.Status.Phase == virtv2.VirtualMachineMACAddressPhaseAttached || existed.Status.VirtualMachine != "" {
		return fmt.Errorf("the virtual machine mac address %q is %w and cannot be used for the restored virtual machine", vmmacKey.Name, ErrAlreadyInUse)
	}

	return nil
}

func (v *VirtualMachineMACAddressOverrideValidator) ProcessWithForce(_ context.Context) error {
	return nil
}

func (v *VirtualMachineMACAddressOverrideValidator) Object() client.Object {
	return &virtv2.VirtualMachineMACAddress{
		TypeMeta: metav1.TypeMeta{
			Kind:       v.vmmac.Kind,
			APIVersion: v.vmmac.APIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        v.vmmac.Name,
			Namespace:   v.vmmac.Namespace,
			Annotations: v.vmmac.Annotations,
			Labels:      v.vmmac.Labels,
		},
		Spec: v.vmmac.Spec,
	}
}
