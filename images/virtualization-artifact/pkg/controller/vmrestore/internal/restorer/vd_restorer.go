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

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualDiskOverrideValidator struct {
	vd           *v1alpha2.VirtualDisk
	client       client.Client
	vmRestoreUID string
}

func NewVirtualDiskOverrideValidator(vdTmpl *v1alpha2.VirtualDisk, client client.Client, vmRestoreUID string) *VirtualDiskOverrideValidator {
	if vdTmpl.Annotations != nil {
		vdTmpl.Annotations[annotations.AnnVMRestore] = vmRestoreUID
	} else {
		vdTmpl.Annotations = make(map[string]string)
		vdTmpl.Annotations[annotations.AnnVMRestore] = vmRestoreUID
	}
	return &VirtualDiskOverrideValidator{
		vd: &v1alpha2.VirtualDisk{
			TypeMeta: metav1.TypeMeta{
				Kind:       vdTmpl.Kind,
				APIVersion: vdTmpl.APIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        vdTmpl.Name,
				Namespace:   vdTmpl.Namespace,
				Annotations: vdTmpl.Annotations,
				Labels:      vdTmpl.Labels,
			},
			Spec:   vdTmpl.Spec,
			Status: vdTmpl.Status,
		},
		client:       client,
		vmRestoreUID: vmRestoreUID,
	}
}

func (v *VirtualDiskOverrideValidator) Override(rules []v1alpha2.NameReplacement) {
	v.vd.Name = overrideName(v.vd.Kind, v.vd.Name, rules)
}

func (v *VirtualDiskOverrideValidator) Validate(ctx context.Context) error {
	vdKey := types.NamespacedName{Namespace: v.vd.Namespace, Name: v.vd.Name}
	existed, err := object.FetchObject(ctx, vdKey, v.client, &v1alpha2.VirtualDisk{})
	if err != nil {
		return err
	}

	if existed != nil {
		if value, ok := existed.Annotations[annotations.AnnVMRestore]; ok && value == v.vmRestoreUID {
			return nil
		}
		return fmt.Errorf("the virtual disk %q %w", vdKey.Name, ErrAlreadyExists)
	}

	return nil
}

func (v *VirtualDiskOverrideValidator) ValidateWithForce(ctx context.Context) error {
	vdKey := types.NamespacedName{Namespace: v.vd.Namespace, Name: v.vd.Name}
	existed, err := object.FetchObject(ctx, vdKey, v.client, &v1alpha2.VirtualDisk{})
	if err != nil {
		return err
	}

	vmName := v.getVirtualMachineName()

	if existed != nil {
		for _, a := range existed.Status.AttachedToVirtualMachines {
			if a.Mounted && a.Name != vmName {
				return fmt.Errorf("the virtual disk %q %w", existed.Name, ErrAlreadyInUse)
			}
		}
	}

	return nil
}

func (v *VirtualDiskOverrideValidator) ProcessWithForce(ctx context.Context) error {
	vdKey := types.NamespacedName{Namespace: v.vd.Namespace, Name: v.vd.Name}
	vdObj, err := object.FetchObject(ctx, vdKey, v.client, &v1alpha2.VirtualDisk{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `VirtualDisk`: %w", err)
	}

	if object.IsTerminating(vdObj) {
		return fmt.Errorf("waiting for the `VirtualDisk` %s %w", vdObj.Name, ErrRestoring)
	}

	if vdObj != nil {
		if value, ok := vdObj.Annotations[annotations.AnnVMRestore]; ok && value == v.vmRestoreUID {
			return nil
		}
		err := v.client.Delete(ctx, vdObj)
		if err != nil {
			return fmt.Errorf("failed to delete the `VirtualDisk`: %w", err)
		}
		return fmt.Errorf("waiting for the `VirtualDisk` %s %w", vdObj.Name, ErrRestoring)
	}

	return nil
}

func (v *VirtualDiskOverrideValidator) Object() client.Object {
	return &v1alpha2.VirtualDisk{
		TypeMeta: metav1.TypeMeta{
			Kind:       v.vd.Kind,
			APIVersion: v.vd.APIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        v.vd.Name,
			Namespace:   v.vd.Namespace,
			Annotations: v.vd.Annotations,
			Labels:      v.vd.Labels,
		},
		Spec: v.vd.Spec,
	}
}

func (v *VirtualDiskOverrideValidator) getVirtualMachineName() string {
	for _, a := range v.vd.Status.AttachedToVirtualMachines {
		if a.Mounted {
			return a.Name
		}
	}
	return ""
}
