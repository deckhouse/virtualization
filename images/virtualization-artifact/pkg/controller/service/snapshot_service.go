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

package service

import (
	"context"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// RequestFSFreeze is the value of the filesystem request annotation for a freeze that a snapshot started
// before the module update may have left pending on the virtual machine instance.
const RequestFSFreeze = "freeze"

// FSFrozen is the fsFreezeStatus of a virtual machine instance whose guest filesystems are frozen.
const FSFrozen = "frozen"

type SnapshotService struct {
	client     Client
	protection *ProtectionService
}

func NewSnapshotService(client Client, protection *ProtectionService) *SnapshotService {
	return &SnapshotService{
		client:     client,
		protection: protection,
	}
}

func (s *SnapshotService) CreateVolumeSnapshot(ctx context.Context, vs *vsv1.VolumeSnapshot) (*vsv1.VolumeSnapshot, error) {
	annotations.AddAnnotation(vs, annotations.AnnAllowDelete, "true")

	err := s.client.Create(ctx, vs)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, err
	}

	err = s.protection.AddProtection(ctx, vs)
	if err != nil {
		return nil, err
	}

	return vs, nil
}

func (s *SnapshotService) UpdateVolumeSnapshot(ctx context.Context, vs *vsv1.VolumeSnapshot) error {
	return s.client.Update(ctx, vs)
}

func (s *SnapshotService) DeleteVolumeSnapshot(ctx context.Context, vs *vsv1.VolumeSnapshot) error {
	err := s.protection.RemoveProtection(ctx, vs)
	if err != nil {
		return err
	}

	err = EnsureVolumeSnapshotDeletable(ctx, s.client, vs)
	if err != nil {
		return err
	}

	err = s.client.Delete(ctx, vs)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	return nil
}

func (s *SnapshotService) GetVirtualDisk(ctx context.Context, name, namespace string) (*v1alpha2.VirtualDisk, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &v1alpha2.VirtualDisk{})
}

func (s *SnapshotService) GetPersistentVolumeClaim(ctx context.Context, name, namespace string) (*corev1.PersistentVolumeClaim, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &corev1.PersistentVolumeClaim{})
}

func (s *SnapshotService) GetVirtualDiskSnapshot(ctx context.Context, name, namespace string) (*v1alpha2.VirtualDiskSnapshot, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &v1alpha2.VirtualDiskSnapshot{})
}

func (s *SnapshotService) GetVirtualMachine(ctx context.Context, name, namespace string) (*v1alpha2.VirtualMachine, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &v1alpha2.VirtualMachine{})
}

// GetVolumeSnapshot treats a missing snapshot.storage.k8s.io API as a missing VolumeSnapshot: with the
// CRDs removed the VolumeSnapshots are gone too, and a VirtualDiskSnapshot must still be deletable.
func (s *SnapshotService) GetVolumeSnapshot(ctx context.Context, name, namespace string) (*vsv1.VolumeSnapshot, error) {
	vs, err := object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &vsv1.VolumeSnapshot{})
	if meta.IsNoMatchError(err) {
		return nil, nil
	}

	return vs, err
}

func (s *SnapshotService) GetSecret(ctx context.Context, name, namespace string) (*corev1.Secret, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &corev1.Secret{})
}

func (s *SnapshotService) CreateVirtualDiskSnapshot(ctx context.Context, vdSnapshot *v1alpha2.VirtualDiskSnapshot) (*v1alpha2.VirtualDiskSnapshot, error) {
	err := s.client.Create(ctx, vdSnapshot)
	if err != nil {
		return nil, err
	}

	return vdSnapshot, nil
}

func (s *SnapshotService) GetVirtualMachineInstance(ctx context.Context, vm *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
	if vm == nil {
		return nil, nil
	}

	return object.FetchObject(ctx, client.ObjectKeyFromObject(vm), s.client, &virtv1.VirtualMachineInstance{})
}
