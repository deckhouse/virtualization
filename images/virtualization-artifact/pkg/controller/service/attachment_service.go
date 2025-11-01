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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

type AttachmentService struct {
	client              Client
	virtClient          kubeclient.Client
	controllerNamespace string
}

func NewAttachmentService(client Client, virtClient kubeclient.Client, controllerNamespace string) *AttachmentService {
	return &AttachmentService{
		client:              client,
		virtClient:          virtClient,
		controllerNamespace: controllerNamespace,
	}
}

var (
	ErrVolumeStatusNotReady      = errors.New("hotplug is not ready")
	ErrBlockDeviceIsSpecAttached = errors.New("block device is already attached to the virtual machine spec")
	ErrHotPlugRequestAlreadySent = errors.New("attachment request is already sent")
)

func (s AttachmentService) IsHotPlugged(ad *AttachmentDisk, vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance) (bool, error) {
	if ad == nil {
		return false, errors.New("cannot check if a empty AttachmentDisk is hot plugged")
	}

	if vm == nil {
		return false, errors.New("cannot check if a disk is hot plugged into a nil VirtualMachine")
	}

	if kvvmi == nil {
		return false, errors.New("cannot check if a disk is hot plugged into a nil KVVMI")
	}

	for _, vs := range kvvmi.Status.VolumeStatus {
		if vs.HotplugVolume != nil && vs.Name == ad.GenerateName {
			if vs.Phase == virtv1.VolumeReady {
				return true, nil
			}

			return false, fmt.Errorf("%w: %s", ErrVolumeStatusNotReady, vs.Message)
		}
	}

	return false, nil
}

func (s AttachmentService) CanHotPlug(ad *AttachmentDisk, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) (bool, error) {
	if ad == nil {
		return false, errors.New("cannot hot plug a nil AttachmentDisk")
	}

	if vm == nil {
		return false, errors.New("cannot hot plug a disk into a nil VirtualMachine")
	}

	if kvvm == nil {
		return false, errors.New("cannot hot plug a disk into a nil KVVM")
	}

	for _, bdr := range vm.Spec.BlockDeviceRefs {
		if bdr.Kind == ad.Kind && bdr.Name == ad.Name {
			return false, fmt.Errorf("%w: virtual machine has a block device reference, but it is not a hot-plugged volume", ErrBlockDeviceIsSpecAttached)
		}
	}

	name := ad.GenerateName

	if kvvm.Spec.Template != nil {
		for _, vs := range kvvm.Spec.Template.Spec.Volumes {
			if vs.Name == name {
				switch {
				case vs.PersistentVolumeClaim != nil:
					if !vs.PersistentVolumeClaim.Hotpluggable {
						return false, fmt.Errorf("%w: virtual machine has a block device reference, but it is not a hot-plugged volume", ErrBlockDeviceIsSpecAttached)
					}
				case vs.ContainerDisk != nil:
					if !vs.ContainerDisk.Hotpluggable {
						return false, fmt.Errorf("%w: virtual machine has a block device reference, but it is not a hot-plugged volume", ErrBlockDeviceIsSpecAttached)
					}
				default:
					return false, fmt.Errorf("kvvm %s/%s spec volume %s does not have a pvc or container disk reference", kvvm.Namespace, kvvm.Name, vs.Name)
				}

				return false, ErrHotPlugRequestAlreadySent
			}
		}
	}

	for _, vr := range kvvm.Status.VolumeRequests {
		if vr.AddVolumeOptions.Name == name {
			return false, ErrHotPlugRequestAlreadySent
		}
	}

	return true, nil
}

func (s AttachmentService) HotPlugDisk(ctx context.Context, ad *AttachmentDisk, vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine) error {
	if ad == nil {
		return errors.New("cannot hot plug a nil AttachmentDisk")
	}

	if vm == nil {
		return errors.New("cannot hot plug a disk into a nil VirtualMachine")
	}

	if kvvm == nil {
		return errors.New("cannot hot plug a disk into a nil KVVM")
	}

	return s.virtClient.VirtualMachines(vm.GetNamespace()).AddVolume(ctx, vm.GetName(), subv1alpha2.VirtualMachineAddVolume{
		VolumeKind: string(ad.Kind),
		Name:       ad.GenerateName,
		Image:      ad.Image,
		PVCName:    ad.PVCName,
		Serial:     ad.Serial,
		IsCdrom:    ad.IsCdrom,
	})
}

func (s AttachmentService) IsAttached(vm *v1alpha2.VirtualMachine, kvvm *virtv1.VirtualMachine, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) bool {
	if vm == nil || kvvm == nil {
		return false
	}

	for _, bdRef := range vm.Status.BlockDeviceRefs {
		if bdRef.Kind == v1alpha2.BlockDeviceKind(vmbda.Spec.BlockDeviceRef.Kind) && bdRef.Name == vmbda.Spec.BlockDeviceRef.Name {
			return bdRef.Hotplugged && bdRef.VirtualMachineBlockDeviceAttachmentName == vmbda.Name
		}
	}

	return false
}

func (s AttachmentService) UnplugDisk(ctx context.Context, kvvm *virtv1.VirtualMachine, diskName string) error {
	if kvvm == nil {
		return errors.New("cannot unplug a disk from a nil KVVM")
	}
	if diskName == "" {
		return errors.New("cannot unplug a disk with a empty DiskName")
	}
	return s.virtClient.VirtualMachines(kvvm.GetNamespace()).RemoveVolume(ctx, kvvm.GetName(), subv1alpha2.VirtualMachineRemoveVolume{
		Name: diskName,
	})
}

// IsConflictedAttachment returns true if the provided VMBDA conflicts with another
// previously created or started VMBDA with the same disk.
// There should be no other Non-Conflicted VMBDAs.
//
// Examples: Check if VMBDA A is conflicted:
//
// T1: -->VMBDA A Should be Conflicted
// T1:    VMBDA B Phase: "Attached"
//
// T1: -->VMBDA A Should be Non-Conflicted
// T1:    VMBDA B Phase: "Failed"
//
// T1:    VMBDA B Phase: ""
// T2: -->VMBDA A Should be Conflicted
//
// T1: -->VMBDA A Should be Non-Conflicted
// T2:    VMBDA B Phase: ""
//
// T1: -->VMBDA A Should be Non-Conflicted lexicographically
// T1:    VMBDA B Phase: ""
func (s AttachmentService) IsConflictedAttachment(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) (bool, string, error) {
	// CVI and VI always has no conflicts. Skip
	if vmbda.Spec.BlockDeviceRef.Kind == v1alpha2.ClusterVirtualImageKind || vmbda.Spec.BlockDeviceRef.Kind == v1alpha2.VirtualImageKind {
		return false, "", nil
	}

	var vmbdas v1alpha2.VirtualMachineBlockDeviceAttachmentList
	err := s.client.List(ctx, &vmbdas, &client.ListOptions{Namespace: vmbda.Namespace})
	if err != nil {
		return false, "", err
	}

	for i := range vmbdas.Items {
		// If the virtual machine and disk do not match, there is no conflict with this VMBDA.
		if vmbdas.Items[i].Name == vmbda.Name || !isSameBlockDeviceRefs(vmbdas.Items[i].Spec.BlockDeviceRef, vmbda.Spec.BlockDeviceRef) {
			continue
		}

		// There is already a Non-Conflicted VMBDA.
		if vmbdas.Items[i].Status.Phase != "" && vmbdas.Items[i].Status.Phase != v1alpha2.BlockDeviceAttachmentPhaseFailed {
			return true, vmbdas.Items[i].Name, nil
		}

		switch vmbdas.Items[i].CreationTimestamp.Time.Compare(vmbda.CreationTimestamp.Time) {
		case -1:
			// The current VMBDA undergoing reconciliation conflicts with another previously created VMBDA.
			return true, vmbdas.Items[i].Name, nil
		case 0:
			// Same creation time, the earliest lexicographically should be processed, the others are considered conflicting.
			if strings.Compare(vmbdas.Items[i].Name, vmbda.Name) == -1 {
				return true, vmbdas.Items[i].Name, nil
			}
		case 1:
		}
	}

	return false, "", nil
}

func (s AttachmentService) GetVirtualDisk(ctx context.Context, name, namespace string) (*v1alpha2.VirtualDisk, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &v1alpha2.VirtualDisk{})
}

func (s AttachmentService) GetVirtualImage(ctx context.Context, name, namespace string) (*v1alpha2.VirtualImage, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &v1alpha2.VirtualImage{})
}

func (s AttachmentService) GetClusterVirtualImage(ctx context.Context, name string) (*v1alpha2.ClusterVirtualImage, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: name}, s.client, &v1alpha2.ClusterVirtualImage{})
}

func (s AttachmentService) GetPersistentVolumeClaim(ctx context.Context, ad *AttachmentDisk) (*corev1.PersistentVolumeClaim, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: ad.Namespace, Name: ad.PVCName}, s.client, &corev1.PersistentVolumeClaim{})
}

func (s AttachmentService) GetPersistentVolume(ctx context.Context, pvName string) (*corev1.PersistentVolume, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: pvName}, s.client, &corev1.PersistentVolume{})
}

func (s AttachmentService) GetVirtualMachine(ctx context.Context, name, namespace string) (*v1alpha2.VirtualMachine, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: namespace, Name: name}, s.client, &v1alpha2.VirtualMachine{})
}

func (s AttachmentService) GetKVVM(ctx context.Context, vm *v1alpha2.VirtualMachine) (*virtv1.VirtualMachine, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, s.client, &virtv1.VirtualMachine{})
}

func (s AttachmentService) GetKVVMI(ctx context.Context, vm *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
	return object.FetchObject(ctx, types.NamespacedName{Namespace: vm.Namespace, Name: vm.Name}, s.client, &virtv1.VirtualMachineInstance{})
}

func (s AttachmentService) IsPVAvailableOnVMNode(ctx context.Context, pvc *corev1.PersistentVolumeClaim, kvvmi *virtv1.VirtualMachineInstance) (bool, error) {
	if pvc == nil {
		return false, errors.New("pvc is nil")
	}
	if kvvmi == nil {
		return false, errors.New("kvvmi is nil")
	}
	if pvc.Spec.VolumeName == "" || kvvmi.Status.NodeName == "" {
		return true, nil
	}

	pv, err := s.GetPersistentVolume(ctx, pvc.Spec.VolumeName)
	if err != nil {
		return false, fmt.Errorf("failed to get PersistentVolume %q: %w", pvc.Spec.VolumeName, err)
	}
	if pv == nil {
		return false, fmt.Errorf("PersistentVolume %q not found", pvc.Spec.VolumeName)
	}

	if pv.Spec.NodeAffinity == nil || pv.Spec.NodeAffinity.Required == nil {
		return true, nil
	}

	nodeName := kvvmi.Status.NodeName
	node := &corev1.Node{}
	err = s.client.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	if err != nil {
		return false, fmt.Errorf("failed to get Node %q: %w", nodeName, err)
	}

	selector, err := nodeaffinity.NewNodeSelector(pv.Spec.NodeAffinity.Required)
	if err != nil {
		return false, fmt.Errorf("failed to get node selector: %w", err)
	}

	if !selector.Match(node) {
		return false, nil
	}

	return true, nil
}

func isSameBlockDeviceRefs(a, b v1alpha2.VMBDAObjectRef) bool {
	return a.Kind == b.Kind && a.Name == b.Name
}

type AttachmentDisk struct {
	Kind         v1alpha2.BlockDeviceKind
	Name         string
	Namespace    string
	GenerateName string
	PVCName      string
	Image        string
	Serial       string
	IsCdrom      bool
}

func NewAttachmentDiskFromVirtualDisk(vd *v1alpha2.VirtualDisk) *AttachmentDisk {
	return &AttachmentDisk{
		Kind:         v1alpha2.DiskDevice,
		Name:         vd.GetName(),
		Namespace:    vd.GetNamespace(),
		GenerateName: kvbuilder.GenerateVDDiskName(vd.GetName()),
		Serial:       kvbuilder.GenerateSerialFromObject(vd),
		PVCName:      vd.Status.Target.PersistentVolumeClaim,
	}
}

func NewAttachmentDiskFromVirtualImage(vi *v1alpha2.VirtualImage) *AttachmentDisk {
	serial := ""
	if !vi.Status.CDROM {
		serial = kvbuilder.GenerateSerialFromObject(vi)
	}
	ad := AttachmentDisk{
		Kind:         v1alpha2.ImageDevice,
		Name:         vi.GetName(),
		Namespace:    vi.GetNamespace(),
		GenerateName: kvbuilder.GenerateVIDiskName(vi.GetName()),
		Serial:       serial,
		IsCdrom:      vi.Status.CDROM,
	}

	if vi.Spec.Storage == v1alpha2.StorageContainerRegistry {
		ad.Image = vi.Status.Target.RegistryURL
	} else {
		ad.PVCName = vi.Status.Target.PersistentVolumeClaim
	}

	return &ad
}

func NewAttachmentDiskFromClusterVirtualImage(cvi *v1alpha2.ClusterVirtualImage) *AttachmentDisk {
	serial := ""
	if !cvi.Status.CDROM {
		serial = kvbuilder.GenerateSerialFromObject(cvi)
	}
	return &AttachmentDisk{
		Kind:         v1alpha2.ClusterImageDevice,
		Name:         cvi.GetName(),
		GenerateName: kvbuilder.GenerateCVIDiskName(cvi.GetName()),
		Image:        cvi.Status.Target.RegistryURL,
		Serial:       serial,
		IsCdrom:      cvi.Status.CDROM,
	}
}
