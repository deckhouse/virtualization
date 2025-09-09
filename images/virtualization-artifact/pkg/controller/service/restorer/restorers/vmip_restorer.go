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

type VirtualMachineIPHandler struct {
	vmip       *v1alpha2.VirtualMachineIPAddress
	client     client.Client
	restoreUID string
}

func NewVirtualMachineIPAddressHandler(client client.Client, vmipTmpl *v1alpha2.VirtualMachineIPAddress, vmRestoreUID string) *VirtualMachineIPHandler {
	if vmipTmpl.Annotations != nil {
		vmipTmpl.Annotations[annotations.AnnVMRestore] = vmRestoreUID
	} else {
		vmipTmpl.Annotations = make(map[string]string)
		vmipTmpl.Annotations[annotations.AnnVMRestore] = vmRestoreUID
	}
	return &VirtualMachineIPHandler{
		vmip: &v1alpha2.VirtualMachineIPAddress{
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
			Spec:   vmipTmpl.Spec,
			Status: vmipTmpl.Status,
		},
		client:     client,
		restoreUID: vmRestoreUID,
	}
}

func (v *VirtualMachineIPHandler) Override(rules []v1alpha2.NameReplacement) {
	v.vmip.Name = common.OverrideName(v.vmip.Kind, v.vmip.Name, rules)
}

func (v *VirtualMachineIPHandler) ValidateRestore(ctx context.Context) error {
	vmipKey := types.NamespacedName{Namespace: v.vmip.Namespace, Name: v.vmip.Name}
	existed, err := object.FetchObject(ctx, vmipKey, v.client, &v1alpha2.VirtualMachineIPAddress{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMRestore]; ok && value == v.restoreUID {
			return nil
		}
	}

	if v.vmip.Spec.StaticIP != "" {
		var vmips v1alpha2.VirtualMachineIPAddressList
		err = v.client.List(ctx, &vmips, &client.ListOptions{
			Namespace:     v.vmip.Namespace,
			FieldSelector: fields.OneTermEqualSelector(indexer.IndexFieldVMIPByAddress, v.vmip.Spec.StaticIP),
		})
		if err != nil {
			return err
		}

		for _, vmip := range vmips.Items {
			if vmip.Status.VirtualMachine == v.vmip.Status.VirtualMachine || vmip.Name == v.vmip.Name {
				continue
			}

			return fmt.Errorf(
				"the set address %q is %w by the different virtual machine ip address %q and cannot be used for the restored virtual machine",
				v.vmip.Spec.StaticIP, common.ErrAlreadyInUse, vmip.Name,
			)
		}
	}

	if existed != nil {
		if existed.Status.Phase == v1alpha2.VirtualMachineIPAddressPhaseAttached && existed.Status.VirtualMachine != v.vmip.Status.VirtualMachine {
			return fmt.Errorf("the virtual machine ip address %q is %w and cannot be used for the restored virtual machine", vmipKey.Name, common.ErrAlreadyInUse)
		}
	}

	return nil
}

func (v *VirtualMachineIPHandler) ProcessRestore(ctx context.Context) error {
	err := v.ValidateRestore(ctx)
	if err != nil {
		return err
	}

	vmipKey := types.NamespacedName{Namespace: v.vmip.Namespace, Name: v.vmip.Name}
	existed, err := object.FetchObject(ctx, vmipKey, v.client, &v1alpha2.VirtualMachineIPAddress{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMRestore]; ok && value == v.restoreUID {
			return nil
		}
	} else {
		err = v.client.Create(ctx, v.vmip)
		if err != nil {
			return fmt.Errorf("failed to create the `VirtualMachineIPAddress`: %w", err)
		}
	}

	return nil
}

func (v *VirtualMachineIPHandler) ValidateClone(ctx context.Context) error {
	return nil
}

func (v *VirtualMachineIPHandler) ProcessClone(ctx context.Context) error {
	return nil
}

func (v *VirtualMachineIPHandler) Object() client.Object {
	return &v1alpha2.VirtualMachineIPAddress{
		TypeMeta: metav1.TypeMeta{
			Kind:       v.vmip.Kind,
			APIVersion: v.vmip.APIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        v.vmip.Name,
			Namespace:   v.vmip.Namespace,
			Annotations: v.vmip.Annotations,
			Labels:      v.vmip.Labels,
		},
		Spec: v.vmip.Spec,
	}
}
