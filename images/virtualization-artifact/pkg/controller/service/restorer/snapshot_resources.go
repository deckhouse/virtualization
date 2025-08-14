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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	restorer "github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/restorers"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type SnapshotResources struct {
	uuid           string
	client         client.Client
	restorer       *SecretRestorer
	restorerSecret *corev1.Secret
	vmSnapshot     *virtv2.VirtualMachineSnapshot
	objectHandlers []ObjectHandler
}

func NewSnapshotResources(client client.Client, restorerSecret *corev1.Secret, vmSnapshot *virtv2.VirtualMachineSnapshot, uuid string) SnapshotResources {
	return SnapshotResources{
		uuid:           uuid,
		client:         client,
		restorer:       NewSecretRestorer(client),
		vmSnapshot:     vmSnapshot,
		restorerSecret: restorerSecret,
	}
}

func (r *SnapshotResources) GetResourcesForRestore(ctx context.Context) error {
	provisioner, err := r.restorer.RestoreProvisioner(ctx, r.restorerSecret)
	if err != nil {
		return err
	}

	vm, err := r.restorer.RestoreVirtualMachine(ctx, r.restorerSecret)
	if err != nil {
		return err
	}

	vmip, err := r.restorer.RestoreVirtualMachineIPAddress(ctx, r.restorerSecret)
	if err != nil {
		return err
	}

	if vmip != nil {
		vm.Spec.VirtualMachineIPAddress = vmip.Name
	}

	vds, err := getVirtualDisks(ctx, r.client, r.vmSnapshot)
	if err != nil {
		return err
	}

	vmbdas, err := r.restorer.RestoreVirtualMachineBlockDeviceAttachments(ctx, r.restorerSecret)
	if err != nil {
		return err
	}

	for _, vd := range vds {
		r.objectHandlers = append(r.objectHandlers, restorer.NewVDHandler(vd, r.client, r.uuid))
	}

	for _, vmbda := range vmbdas {
		r.objectHandlers = append(r.objectHandlers, restorer.NewVMBDAHandler(vmbda, r.client, r.uuid))
	}

	if provisioner != nil {
		r.objectHandlers = append(r.objectHandlers, restorer.NewProvisionerHandler(provisioner, r.client, r.uuid))
	}

	r.objectHandlers = append(r.objectHandlers, restorer.NewVMHandler(vm, r.client, string(r.vmSnapshot.UID)))

	return nil
}

func (r *SnapshotResources) ValidateWithForce(ctx context.Context) error {
	for _, ov := range r.objectHandlers {
		err := ov.ValidateWithForce(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *SnapshotResources) Validate(ctx context.Context) error {
	for _, ov := range r.objectHandlers {
		err := ov.Validate(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *SnapshotResources) Process(ctx context.Context) error {
	for _, ov := range r.objectHandlers {
		err := ov.Process(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *SnapshotResources) ProcessWithForce(ctx context.Context) error {
	for _, ov := range r.objectHandlers {
		err := ov.ProcessWithForce(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func getVirtualDisks(ctx context.Context, client client.Client, vmSnapshot *virtv2.VirtualMachineSnapshot) ([]*virtv2.VirtualDisk, error) {
	vds := make([]*virtv2.VirtualDisk, 0, len(vmSnapshot.Status.VirtualDiskSnapshotNames))

	for _, vdSnapshotName := range vmSnapshot.Status.VirtualDiskSnapshotNames {
		vdSnapshotKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vdSnapshotName}
		vdSnapshot, err := object.FetchObject(ctx, vdSnapshotKey, client, &virtv2.VirtualDiskSnapshot{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch the virtual disk snapshot %q: %w", vdSnapshotKey.Name, err)
		}

		if vdSnapshot == nil {
			return nil, fmt.Errorf("the virtual disk snapshot %q %w", vdSnapshotName, common.ErrVirtualDiskSnapshotNotFound)
		}

		vd := virtv2.VirtualDisk{
			TypeMeta: metav1.TypeMeta{
				Kind:       virtv2.VirtualDiskKind,
				APIVersion: virtv2.Version,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vdSnapshot.Spec.VirtualDiskName,
				Namespace: vdSnapshot.Namespace,
			},
			Spec: virtv2.VirtualDiskSpec{
				DataSource: &virtv2.VirtualDiskDataSource{
					Type: virtv2.DataSourceTypeObjectRef,
					ObjectRef: &virtv2.VirtualDiskObjectRef{
						Kind: virtv2.VirtualDiskObjectRefKindVirtualDiskSnapshot,
						Name: vdSnapshot.Name,
					},
				},
			},
			Status: virtv2.VirtualDiskStatus{
				AttachedToVirtualMachines: []virtv2.AttachedVirtualMachine{
					{Name: vmSnapshot.Spec.VirtualMachineName, Mounted: true},
				},
			},
		}

		vds = append(vds, &vd)
	}

	return vds, nil
}
