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
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal/snapshot/common"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VDHandler struct {
	vd           *virtv2.VirtualDisk
	client       client.Client
	vmRestoreUID string
}

func NewVDHandler(vdTmpl *virtv2.VirtualDisk, client client.Client, vmRestoreUID string) *VDHandler {
	if vdTmpl.Annotations != nil {
		vdTmpl.Annotations[annotations.AnnVMRestore] = vmRestoreUID
	} else {
		vdTmpl.Annotations = make(map[string]string)
		vdTmpl.Annotations[annotations.AnnVMRestore] = vmRestoreUID
	}
	return &VDHandler{
		vd: &virtv2.VirtualDisk{
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

func (v *VDHandler) Override(rules []virtv2.NameReplacement) {
	v.vd.Name = common.OverrideName(v.vd.Kind, v.vd.Name, rules)
}

func (v *VDHandler) Validate(ctx context.Context) error {
	vdKey := types.NamespacedName{Namespace: v.vd.Namespace, Name: v.vd.Name}
	existed, err := object.FetchObject(ctx, vdKey, v.client, &virtv2.VirtualDisk{})
	if err != nil {
		return err
	}

	if existed != nil {
		return fmt.Errorf("the virtual disk %q %w", vdKey.Name, common.ErrAlreadyExists)
	}

	return nil
}

func (v *VDHandler) ValidateWithForce(ctx context.Context) error {
	vdKey := types.NamespacedName{Namespace: v.vd.Namespace, Name: v.vd.Name}
	existed, err := object.FetchObject(ctx, vdKey, v.client, &virtv2.VirtualDisk{})
	if err != nil {
		return err
	}

	vmName := v.getVirtualMachineName()

	if existed != nil {
		for _, a := range existed.Status.AttachedToVirtualMachines {
			if a.Mounted && a.Name != vmName {
				return fmt.Errorf("the virtual disk %q %w", existed.Name, common.ErrAlreadyInUse)
			}
		}
	}

	return nil
}

func (v *VDHandler) Process(ctx context.Context) error {
	return nil
}

func (v *VDHandler) ProcessWithForce(ctx context.Context) error {
	vdKey := types.NamespacedName{Namespace: v.vd.Namespace, Name: v.vd.Name}
	vdObj, err := object.FetchObject(ctx, vdKey, v.client, &virtv2.VirtualDisk{})
	if err != nil {
		return fmt.Errorf("failed to fetch the `VirtualDisk`: %w", err)
	}

	if object.IsTerminating(vdObj) {
		return fmt.Errorf("waiting for the `VirtualDisk` %s %w", vdObj.Name, common.ErrRestoring)
	}

	if vdObj != nil {
		if value, ok := vdObj.Annotations[annotations.AnnVMRestore]; ok && value == v.vmRestoreUID {
			return nil
		}
		err := v.client.Delete(ctx, vdObj)
		if err != nil {
			return fmt.Errorf("failed to delete the `VirtualDisk`: %w", err)
		}
		return fmt.Errorf("waiting for the `VirtualDisk` %s %w", vdObj.Name, common.ErrRestoring)
	}

	return nil
}

func (v *VDHandler) Object() client.Object {
	return &virtv2.VirtualDisk{
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

func (v *VDHandler) getVirtualMachineName() string {
	for _, a := range v.vd.Status.AttachedToVirtualMachines {
		if a.Mounted {
			return a.Name
		}
	}
	return ""
}
