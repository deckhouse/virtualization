/*
Copyright 2025 Flant JSC

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
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineMACHandler struct {
	vmmac      *v1alpha2.VirtualMachineMACAddress
	client     client.Client
	restoreUID string
}

func NewVirtualMachineMACAddressHandler(client client.Client, vmmacTmpl *v1alpha2.VirtualMachineMACAddress, vmRestoreUID string) *VirtualMachineMACHandler {
	if vmmacTmpl.Annotations != nil {
		vmmacTmpl.Annotations[annotations.AnnVMOPRestore] = vmRestoreUID
	} else {
		vmmacTmpl.Annotations = make(map[string]string)
		vmmacTmpl.Annotations[annotations.AnnVMOPRestore] = vmRestoreUID
	}
	return &VirtualMachineMACHandler{
		vmmac: &v1alpha2.VirtualMachineMACAddress{
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
		client:     client,
		restoreUID: vmRestoreUID,
	}
}

func (v *VirtualMachineMACHandler) Override(rules []v1alpha2.NameReplacement) {
	v.vmmac.Name = common.OverrideName(v.vmmac.Kind, v.vmmac.Name, rules)
}

func (v *VirtualMachineMACHandler) ValidateRestore(ctx context.Context) error {
	vmMacKey := types.NamespacedName{Namespace: v.vmmac.Namespace, Name: v.vmmac.Name}
	existed, err := object.FetchObject(ctx, vmMacKey, v.client, &v1alpha2.VirtualMachineMACAddress{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMOPRestore]; ok && value == v.restoreUID {
			return nil
		}
	}

	if v.vmmac.Spec.Address != "" {
		var vmmacs v1alpha2.VirtualMachineMACAddressList
		err = v.client.List(ctx, &vmmacs, &client.ListOptions{
			Namespace:     v.vmmac.Namespace,
			FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVMMACByAddress, v.vmmac.Spec.Address),
		})
		if err != nil {
			return err
		}

		for _, vmMac := range vmmacs.Items {
			if vmMac.Status.VirtualMachine == v.vmmac.Status.VirtualMachine || vmMac.Name == v.vmmac.Name {
				continue
			}

			return fmt.Errorf(
				"the MAC address %q cannot be used for the restore: it is taken by VirtualMachineMACAddress/%q and %w by the different virtual machine",
				v.vmmac.Spec.Address, vmMac.Name, common.ErrAlreadyInUse,
			)
		}
	}

	if existed != nil {
		if existed.Status.Phase == v1alpha2.VirtualMachineMACAddressPhaseAttached && existed.Status.VirtualMachine != v.vmmac.Status.VirtualMachine {
			return fmt.Errorf("the virtual machine MAC address %q is %w and cannot be used for the restored virtual machine", vmMacKey.Name, common.ErrAlreadyInUse)
		}
	}

	return nil
}

func (v *VirtualMachineMACHandler) ProcessRestore(ctx context.Context) error {
	err := v.ValidateRestore(ctx)
	if err != nil {
		return err
	}

	vmMacKey := types.NamespacedName{Namespace: v.vmmac.Namespace, Name: v.vmmac.Name}
	existed, err := object.FetchObject(ctx, vmMacKey, v.client, &v1alpha2.VirtualMachineMACAddress{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMOPRestore]; ok && value == v.restoreUID {
			return nil
		}
	} else {
		err = v.client.Create(ctx, v.vmmac)
		if err != nil {
			return fmt.Errorf("failed to create the `VirtualMachineMacAddress`: %w", err)
		}
	}

	return nil
}

func (v *VirtualMachineMACHandler) Object() client.Object {
	return &v1alpha2.VirtualMachineMACAddress{
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

func (v *VirtualMachineMACHandler) ValidateClone(ctx context.Context) error {
	return nil
}

func (v *VirtualMachineMACHandler) ProcessClone(ctx context.Context) error {
	return nil
}
