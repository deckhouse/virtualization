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
	"errors"
	"fmt"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

const (
	RequestFSFreeze   = "freeze"
	RequestFSUnfreeze = "unfreeze"
	FSFrozen          = "frozen"
)

var (
	ErrUntrustedFilesystemFrozenCondition = errors.New("the filesystem status cannot be processed correctly")
	ErrUnexpectedFilesystemFrozenRequest  = errors.New("found unexpected filesystem frozen request in the virtual machine annotations")
)

type SnapshotService struct {
	virtClient kubeclient.Client
	client     Client
	protection *ProtectionService
}

func NewSnapshotService(virtClient kubeclient.Client, client Client, protection *ProtectionService) *SnapshotService {
	return &SnapshotService{
		virtClient: virtClient,
		client:     client,
		protection: protection,
	}
}

// IsFrozen checks if a freeze or unfreeze request has been performed
// and returns the "true" fsFreezeStatus if the internal virtual machine instance is "frozen",
// and "false" otherwise.
func (s *SnapshotService) IsFrozen(ctx context.Context, vm *v1alpha2.VirtualMachine) (bool, error) {
	if vm == nil {
		return false, nil
	}

	kvvmi, err := object.FetchObject(ctx, client.ObjectKeyFromObject(vm), s.client, &virtv1.VirtualMachineInstance{})
	if err != nil {
		return false, fmt.Errorf("failed to fetch internal virtual machine instance %s/%s: %w", vm.Namespace, vm.Name, err)
	}

	if kvvmi == nil {
		return false, nil
	}

	if kvvmi.Annotations != nil {
		if r, ok := kvvmi.Annotations[annotations.AnnVMFilesystemFrozenRequest]; ok {
			switch {
			case r == RequestFSFreeze && kvvmi.Status.FSFreezeStatus == FSFrozen:
				err := s.removeAnnFSFreezeRequest(ctx, RequestFSFreeze, kvvmi)
				if err != nil {
					return false, err
				}
				return true, nil
			case r == RequestFSUnfreeze && kvvmi.Status.FSFreezeStatus != FSFrozen:
				err := s.removeAnnFSFreezeRequest(ctx, RequestFSUnfreeze, kvvmi)
				if err != nil {
					return false, err
				}
				return false, nil
			default:
				return false, ErrUntrustedFilesystemFrozenCondition
			}
		}
	}

	return kvvmi.Status.FSFreezeStatus == FSFrozen, nil
}

func (s *SnapshotService) CanFreeze(ctx context.Context, vm *v1alpha2.VirtualMachine) (bool, error) {
	if vm == nil || vm.Status.Phase != v1alpha2.MachineRunning {
		return false, nil
	}

	isFrozen, err := s.IsFrozen(ctx, vm)
	if err != nil {
		return false, err
	}
	if isFrozen {
		return false, nil
	}

	agentReady, _ := conditions.GetCondition(vmcondition.TypeAgentReady, vm.Status.Conditions)

	return agentReady.Status == metav1.ConditionTrue, nil
}

func (s *SnapshotService) Freeze(ctx context.Context, name, namespace string) error {
	kvvmi, err := object.FetchObject(ctx, types.NamespacedName{Name: name, Namespace: namespace}, s.client, &virtv1.VirtualMachineInstance{})
	if err != nil {
		return fmt.Errorf("failed to fetch internal virtual machine instance %s/%s: %w", namespace, name, err)
	}

	if kvvmi.Annotations != nil {
		if _, ok := kvvmi.Annotations[annotations.AnnVMFilesystemFrozenRequest]; ok {
			return ErrUnexpectedFilesystemFrozenRequest
		}
	}

	err = s.annotateWithFSFreezeRequest(ctx, RequestFSFreeze, kvvmi)
	if err != nil {
		return fmt.Errorf("failed to annotate virtual machine with filesystem freeze request: %w", err)
	}

	err = s.virtClient.VirtualMachines(namespace).Freeze(ctx, name, subv1alpha2.VirtualMachineFreeze{})
	if err != nil {
		err := s.removeAnnFSFreezeRequest(ctx, RequestFSFreeze, kvvmi)
		if err != nil {
			return fmt.Errorf("failed to remove virtual machine annotation with filesystem freeze request: %w", err)
		}
		return fmt.Errorf("failed to freeze virtual machine %s/%s: %w", namespace, name, err)
	}

	return nil
}

func (s *SnapshotService) CanUnfreezeWithVirtualDiskSnapshot(ctx context.Context, vdSnapshotName string, vm *v1alpha2.VirtualMachine) (bool, error) {
	if vm == nil {
		return false, nil
	}

	isFrozen, err := s.IsFrozen(ctx, vm)
	if err != nil {
		return false, err
	}

	if !isFrozen {
		return false, nil
	}

	vdByName := make(map[string]struct{})
	for _, bdr := range vm.Status.BlockDeviceRefs {
		if bdr.Kind == v1alpha2.DiskDevice {
			vdByName[bdr.Name] = struct{}{}
		}
	}

	var vdSnapshots v1alpha2.VirtualDiskSnapshotList
	err = s.client.List(ctx, &vdSnapshots, &client.ListOptions{
		Namespace: vm.Namespace,
	})
	if err != nil {
		return false, err
	}

	for _, vdSnapshot := range vdSnapshots.Items {
		if vdSnapshot.Name == vdSnapshotName {
			continue
		}

		_, ok := vdByName[vdSnapshot.Spec.VirtualDiskName]
		if ok && vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseInProgress {
			return false, nil
		}
	}

	var vmSnapshots v1alpha2.VirtualMachineSnapshotList
	err = s.client.List(ctx, &vmSnapshots, &client.ListOptions{
		Namespace: vm.Namespace,
	})
	if err != nil {
		return false, err
	}

	for _, vmSnapshot := range vmSnapshots.Items {
		if vmSnapshot.Spec.VirtualMachineName == vm.Name && vmSnapshot.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseInProgress {
			return false, nil
		}
	}

	return true, nil
}

func (s *SnapshotService) CanUnfreezeWithVirtualMachineSnapshot(ctx context.Context, vmSnapshotName string, vm *v1alpha2.VirtualMachine) (bool, error) {
	if vm == nil {
		return false, nil
	}

	isFrozen, err := s.IsFrozen(ctx, vm)
	if err != nil {
		return false, err
	}
	if !isFrozen {
		return false, nil
	}

	vdByName := make(map[string]struct{})
	for _, bdr := range vm.Status.BlockDeviceRefs {
		if bdr.Kind == v1alpha2.DiskDevice {
			vdByName[bdr.Name] = struct{}{}
		}
	}

	var vdSnapshots v1alpha2.VirtualDiskSnapshotList
	err = s.client.List(ctx, &vdSnapshots, &client.ListOptions{
		Namespace: vm.Namespace,
	})
	if err != nil {
		return false, err
	}

	for _, vdSnapshot := range vdSnapshots.Items {
		_, ok := vdByName[vdSnapshot.Spec.VirtualDiskName]
		if ok && vdSnapshot.Status.Phase == v1alpha2.VirtualDiskSnapshotPhaseInProgress {
			return false, nil
		}
	}

	var vmSnapshots v1alpha2.VirtualMachineSnapshotList
	err = s.client.List(ctx, &vmSnapshots, &client.ListOptions{
		Namespace: vm.Namespace,
	})
	if err != nil {
		return false, err
	}

	for _, vmSnapshot := range vmSnapshots.Items {
		if vmSnapshot.Name == vmSnapshotName {
			continue
		}

		if vmSnapshot.Spec.VirtualMachineName == vm.Name && vmSnapshot.Status.Phase == v1alpha2.VirtualMachineSnapshotPhaseInProgress {
			return false, nil
		}
	}

	return true, nil
}

func (s *SnapshotService) Unfreeze(ctx context.Context, name, namespace string) error {
	kvvmi, err := object.FetchObject(ctx, types.NamespacedName{Name: name, Namespace: namespace}, s.client, &virtv1.VirtualMachineInstance{})
	if err != nil {
		return fmt.Errorf("failed to fetch internal virtual machine instance %s/%s: %w", namespace, name, err)
	}

	if kvvmi.Annotations != nil {
		if _, ok := kvvmi.Annotations[annotations.AnnVMFilesystemFrozenRequest]; ok {
			return ErrUnexpectedFilesystemFrozenRequest
		}
	}

	err = s.annotateWithFSFreezeRequest(ctx, RequestFSUnfreeze, kvvmi)
	if err != nil {
		return fmt.Errorf("failed to annotate internal virtual machine instance with filesystem unfreeze request: %w", err)
	}

	err = s.virtClient.VirtualMachines(namespace).Unfreeze(ctx, name)
	if err != nil {
		err := s.removeAnnFSFreezeRequest(ctx, RequestFSUnfreeze, kvvmi)
		if err != nil {
			return fmt.Errorf("failed to remove internal virtual machine instance annotation with filesystem unfreeze request: %w", err)
		}
		return fmt.Errorf("unfreeze virtual machine %s/%s: %w", namespace, name, err)
	}

	return nil
}

func (s *SnapshotService) CreateVolumeSnapshot(ctx context.Context, vs *vsv1.VolumeSnapshot) (*vsv1.VolumeSnapshot, error) {
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

func (s *SnapshotService) DeleteVolumeSnapshot(ctx context.Context, vs *vsv1.VolumeSnapshot) error {
	err := s.protection.RemoveProtection(ctx, vs)
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

func (s *SnapshotService) GetVolumeSnapshot(ctx context.Context, name, namespace string) (*vsv1.VolumeSnapshot, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &vsv1.VolumeSnapshot{})
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

func (s *SnapshotService) annotateWithFSFreezeRequest(ctx context.Context, requestType string, kvvmi *virtv1.VirtualMachineInstance) error {
	if kvvmi.Annotations == nil {
		kvvmi.Annotations = make(map[string]string)
	}
	kvvmi.Annotations[annotations.AnnVMFilesystemFrozenRequest] = requestType

	err := s.client.Update(ctx, kvvmi)
	if err != nil {
		return err
	}

	return nil
}

func (s *SnapshotService) removeAnnFSFreezeRequest(ctx context.Context, requestType string, kvvmi *virtv1.VirtualMachineInstance) error {
	if kvvmi.Annotations == nil {
		return nil
	}

	if rt, ok := kvvmi.Annotations[annotations.AnnVMFilesystemFrozenRequest]; ok && rt == requestType {
		delete(kvvmi.Annotations, annotations.AnnVMFilesystemFrozenRequest)
	}

	err := s.client.Update(ctx, kvvmi)
	if err != nil {
		return err
	}

	return nil
}
