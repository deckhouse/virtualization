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

type VirtualMachineBlockDeviceAttachmentsOverrideValidator struct {
	vmbda  *virtv2.VirtualMachineBlockDeviceAttachment
	client client.Client
}

func NewVirtualMachineBlockDeviceAttachmentsOverrideValidator(vmbdaTmpl *virtv2.VirtualMachineBlockDeviceAttachment, client client.Client) *VirtualMachineBlockDeviceAttachmentsOverrideValidator {
	return &VirtualMachineBlockDeviceAttachmentsOverrideValidator{
		vmbda: &virtv2.VirtualMachineBlockDeviceAttachment{
			TypeMeta: metav1.TypeMeta{
				Kind:       vmbdaTmpl.Kind,
				APIVersion: vmbdaTmpl.APIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        vmbdaTmpl.Name,
				Namespace:   vmbdaTmpl.Namespace,
				Annotations: vmbdaTmpl.Annotations,
				Labels:      vmbdaTmpl.Labels,
			},
			Spec: vmbdaTmpl.Spec,
		},
		client: client,
	}
}

func (v *VirtualMachineBlockDeviceAttachmentsOverrideValidator) Override(rules []virtv2.NameReplacement) {
	v.vmbda.Name = overrideName(v.vmbda.Kind, v.vmbda.Name, rules)
	v.vmbda.Spec.VirtualMachineName = overrideName(virtv2.VirtualMachineKind, v.vmbda.Spec.VirtualMachineName, rules)

	if v.vmbda.Spec.BlockDeviceRef.Kind == virtv2.VMBDAObjectRefKindVirtualDisk {
		v.vmbda.Spec.BlockDeviceRef.Name = overrideName(virtv2.VirtualDiskKind, v.vmbda.Spec.BlockDeviceRef.Name, rules)
	}
}

func (v *VirtualMachineBlockDeviceAttachmentsOverrideValidator) Validate(ctx context.Context) error {
	vmbdaKey := types.NamespacedName{Namespace: v.vmbda.Namespace, Name: v.vmbda.Name}
	existed, err := object.FetchObject(ctx, vmbdaKey, v.client, &virtv2.VirtualMachineBlockDeviceAttachment{})
	if err != nil {
		return err
	}

	if existed != nil {
		return fmt.Errorf("the virtual machine block device attachment %q %w", vmbdaKey.Name, ErrAlreadyExists)
	}

	return nil
}

func (v *VirtualMachineBlockDeviceAttachmentsOverrideValidator) Object() client.Object {
	return v.vmbda
}
