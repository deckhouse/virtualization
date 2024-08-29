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
	"fmt"
	"strings"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/subresources/v1alpha2"
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

func (s *SnapshotService) IsFrozen(vm *virtv2.VirtualMachine) bool {
	if vm == nil {
		return false
	}

	filesystemReady, _ := GetCondition(vmcondition.TypeFilesystemReady.String(), vm.Status.Conditions)

	return filesystemReady.Status == metav1.ConditionFalse && filesystemReady.Reason == vmcondition.ReasonFilesystemFrozen.String()
}

func (s *SnapshotService) CanFreeze(vm *virtv2.VirtualMachine) bool {
	if vm == nil || vm.Status.Phase != virtv2.MachineRunning || s.IsFrozen(vm) {
		return false
	}

	agentReady, _ := GetCondition(vmcondition.TypeAgentReady.String(), vm.Status.Conditions)

	return agentReady.Status == metav1.ConditionTrue
}

func (s *SnapshotService) Freeze(ctx context.Context, name, namespace string) error {
	err := s.virtClient.VirtualMachines(namespace).Freeze(ctx, name, v1alpha2.VirtualMachineFreeze{})
	if err != nil {
		return fmt.Errorf("failed to freeze virtual machine %s/%s: %w", namespace, name, err)
	}

	return nil
}

func (s *SnapshotService) CanUnfreeze(ctx context.Context, vdSnapshotName string, vm *virtv2.VirtualMachine) (bool, error) {
	if vm == nil || !s.IsFrozen(vm) {
		return false, nil
	}

	vdByName := make(map[string]struct{})
	for _, bdr := range vm.Status.BlockDeviceRefs {
		if bdr.Kind != virtv2.DiskDevice {
			vdByName[bdr.Name] = struct{}{}
		}
	}

	var vdSnapshots virtv2.VirtualDiskSnapshotList
	err := s.client.List(ctx, &vdSnapshots, &client.ListOptions{
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
		if ok {
			return false, nil
		}
	}

	return true, nil
}

func (s *SnapshotService) Unfreeze(ctx context.Context, name, namespace string) error {
	err := s.virtClient.VirtualMachines(namespace).Unfreeze(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to unfreeze internal virtual machine %s/%s: %w", namespace, name, err)
	}

	return nil
}

func (s *SnapshotService) CreateVolumeSnapshot(ctx context.Context, vdSnapshot *virtv2.VirtualDiskSnapshot, pvc *corev1.PersistentVolumeClaim) (*vsv1.VolumeSnapshot, error) {
	anno := make(map[string]string)
	if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "" {
		anno["storageClass"] = *pvc.Spec.StorageClassName
		accessModes := make([]string, 0, len(pvc.Status.AccessModes))
		for _, accessMode := range pvc.Status.AccessModes {
			accessModes = append(accessModes, string(accessMode))
		}

		anno["accessModes"] = strings.Join(accessModes, ",")
	}

	volumeSnapshot := vsv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: anno,
			Name:        vdSnapshot.Name,
			Namespace:   vdSnapshot.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				MakeOwnerReference(vdSnapshot),
			},
		},
		Spec: vsv1.VolumeSnapshotSpec{
			Source: vsv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: &pvc.Name,
			},
			VolumeSnapshotClassName: &vdSnapshot.Spec.VolumeSnapshotClassName,
		},
	}

	err := s.client.Create(ctx, &volumeSnapshot)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return nil, err
	}

	err = s.protection.AddProtection(ctx, &volumeSnapshot)
	if err != nil {
		return nil, err
	}

	return &volumeSnapshot, nil
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

func (s *SnapshotService) GetPersistentVolumeClaim(ctx context.Context, name, namespace string) (*corev1.PersistentVolumeClaim, error) {
	return helper.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &corev1.PersistentVolumeClaim{})
}

func (s *SnapshotService) GetVirtualDisk(ctx context.Context, name, namespace string) (*virtv2.VirtualDisk, error) {
	return helper.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &virtv2.VirtualDisk{})
}

func (s *SnapshotService) GetVirtualMachine(ctx context.Context, name, namespace string) (*virtv2.VirtualMachine, error) {
	return helper.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &virtv2.VirtualMachine{})
}

func (s *SnapshotService) GetVolumeSnapshot(ctx context.Context, name, namespace string) (*vsv1.VolumeSnapshot, error) {
	return helper.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &vsv1.VolumeSnapshot{})
}
