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

package kvbuilder

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/common/pointer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/ipam"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	VMDDiskPrefix        = "vd-"
	VMIDiskPrefix        = "vi-"
	CVMIDiskPrefix       = "cvi-"
	NetworkInterfaceName = "default"
)

func GenerateVMDDiskName(name string) string {
	return VMDDiskPrefix + name
}

func GenerateVMIDiskName(name string) string {
	return VMIDiskPrefix + name
}

func GenerateCVMIDiskName(name string) string {
	return CVMIDiskPrefix + name
}

func GetOriginalDiskName(prefixedName string) (string, virtv2.BlockDeviceKind) {
	switch {
	case strings.HasPrefix(prefixedName, VMDDiskPrefix):
		return strings.TrimPrefix(prefixedName, VMDDiskPrefix), virtv2.DiskDevice
	case strings.HasPrefix(prefixedName, VMIDiskPrefix):
		return strings.TrimPrefix(prefixedName, VMIDiskPrefix), virtv2.ImageDevice
	case strings.HasPrefix(prefixedName, CVMIDiskPrefix):
		return strings.TrimPrefix(prefixedName, CVMIDiskPrefix), virtv2.ClusterImageDevice
	}

	return prefixedName, ""
}

func GenerateSerialFromObject(obj metav1.Object) string {
	return GenerateSerial(string(obj.GetUID()))
}

func GenerateSerial(input string) string {
	hasher := md5.New()
	hasher.Write([]byte(input))
	hashInBytes := hasher.Sum(nil)
	return hex.EncodeToString(hashInBytes)
}

type HotPlugDeviceSettings struct {
	VolumeName     string
	PVCName        string
	DataVolumeName string
}

func ApplyVirtualMachineSpec(
	kvvm *KVVM, vm *virtv2.VirtualMachine,
	vdByName map[string]*virtv2.VirtualDisk,
	viByName map[string]*virtv2.VirtualImage,
	cviByName map[string]*virtv2.ClusterVirtualImage,
	class *virtv2.VirtualMachineClass,
	ipAddress string,
) error {
	if err := kvvm.SetRunPolicy(vm.Spec.RunPolicy); err != nil {
		return err
	}
	if err := kvvm.SetOsType(vm.Spec.OsType); err != nil {
		return err
	}
	if err := kvvm.SetBootloader(vm.Spec.Bootloader); err != nil {
		return err
	}
	if err := kvvm.SetCPUModel(class); err != nil {
		return err
	}

	kvvm.SetMetadata(vm.ObjectMeta)
	kvvm.SetNetworkInterface(NetworkInterfaceName)
	kvvm.SetTablet("default-0")
	kvvm.SetNodeSelector(vm.Spec.NodeSelector, class.Spec.NodeSelector.MatchLabels)
	kvvm.SetTolerations(vm.Spec.Tolerations, class.Spec.Tolerations)
	kvvm.SetAffinity(virtv2.NewAffinityFromVMAffinity(vm.Spec.Affinity), class.Spec.NodeSelector.MatchExpressions)
	kvvm.SetPriorityClassName(vm.Spec.PriorityClassName)
	kvvm.SetTerminationGracePeriod(vm.Spec.TerminationGracePeriodSeconds)
	kvvm.SetTopologySpreadConstraint(vm.Spec.TopologySpreadConstraints)
	kvvm.SetMemory(vm.Spec.Memory.Size)
	if err := kvvm.SetCPU(vm.Spec.CPU.Cores, vm.Spec.CPU.CoreFraction); err != nil {
		return err
	}

	hotpluggedDevices := make([]HotPlugDeviceSettings, 0)
	for _, volume := range kvvm.Resource.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.Hotpluggable {
			hotpluggedDevices = append(hotpluggedDevices, HotPlugDeviceSettings{
				VolumeName: volume.Name,
				PVCName:    volume.PersistentVolumeClaim.ClaimName,
			})
		}
		// FIXME(VM): not used, now only supports PVC
		if volume.DataVolume != nil && volume.DataVolume.Hotpluggable {
			hotpluggedDevices = append(hotpluggedDevices, HotPlugDeviceSettings{
				VolumeName:     volume.Name,
				DataVolumeName: volume.DataVolume.Name,
			})
		}
	}

	kvvm.ClearDisks()
	bootOrder := uint(1)
	for _, bd := range vm.Spec.BlockDeviceRefs {
		// bootOrder starts from 1.
		switch bd.Kind {
		case virtv2.ImageDevice:
			// Attach ephemeral disk for storage: Kubernetes.
			// Attach containerDisk for storage: ContainerRegistry (i.e. image from DVCR).

			vi := viByName[bd.Name]

			name := GenerateVMIDiskName(bd.Name)
			switch vi.Spec.Storage {
			case virtv2.StorageKubernetes,
				virtv2.StoragePersistentVolumeClaim:
				// Attach PVC as ephemeral volume: its data will be restored to initial state on VM restart.
				if err := kvvm.SetDisk(name, SetDiskOptions{
					PersistentVolumeClaim: pointer.GetPointer(vi.Status.Target.PersistentVolumeClaim),
					IsEphemeral:           true,
					Serial:                GenerateSerialFromObject(vi),
					BootOrder:             bootOrder,
				}); err != nil {
					return err
				}
			case virtv2.StorageContainerRegistry:
				if err := kvvm.SetDisk(name, SetDiskOptions{
					ContainerDisk: pointer.GetPointer(vi.Status.Target.RegistryURL),
					IsCdrom:       imageformat.IsISO(vi.Status.Format),
					Serial:        GenerateSerialFromObject(vi),
					BootOrder:     bootOrder,
				}); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unexpected storage type %q for vi %s. %w", vi.Spec.Storage, vi.Name, common.ErrUnknownType)
			}
			bootOrder++

		case virtv2.ClusterImageDevice:
			// ClusterVirtualImage is attached as containerDisk.

			cvi := cviByName[bd.Name]

			name := GenerateCVMIDiskName(bd.Name)
			if err := kvvm.SetDisk(name, SetDiskOptions{
				ContainerDisk: pointer.GetPointer(cvi.Status.Target.RegistryURL),
				IsCdrom:       imageformat.IsISO(cvi.Status.Format),
				Serial:        GenerateSerialFromObject(cvi),
				BootOrder:     bootOrder,
			}); err != nil {
				return err
			}
			bootOrder++

		case virtv2.DiskDevice:
			// VirtualDisk is attached as a regular disk.

			vd := vdByName[bd.Name]
			// VirtualDisk doesn't have pvc yet: wait for pvc and reconcile again.
			if vd.Status.Target.PersistentVolumeClaim == "" {
				continue
			}

			name := GenerateVMDDiskName(bd.Name)
			if err := kvvm.SetDisk(name, SetDiskOptions{
				PersistentVolumeClaim: pointer.GetPointer(vd.Status.Target.PersistentVolumeClaim),
				Serial:                GenerateSerialFromObject(vd),
				BootOrder:             bootOrder,
			}); err != nil {
				return err
			}
			bootOrder++
		default:
			return fmt.Errorf("unknown block device kind %q. %w", bd.Kind, common.ErrUnknownType)
		}
	}

	for _, device := range hotpluggedDevices {
		switch {
		case device.PVCName != "":
			if err := kvvm.SetDisk(device.VolumeName, SetDiskOptions{
				PersistentVolumeClaim: pointer.GetPointer(device.PVCName),
				IsHotplugged:          true,
			}); err != nil {
				return err
			}
			// FIXME(VM): not used, now only supports PVC
		case device.DataVolumeName != "":
		}
	}
	if err := kvvm.SetProvisioning(vm.Spec.Provisioning); err != nil {
		return err
	}

	kvvm.SetOwnerRef(vm, schema.GroupVersionKind{
		Group:   virtv2.SchemeGroupVersion.Group,
		Version: virtv2.SchemeGroupVersion.Version,
		Kind:    "VirtualMachine",
	})
	kvvm.AddFinalizer(virtv2.FinalizerKVVMProtection)

	// Set ip address cni request annotation.
	kvvm.SetKVVMIAnnotation(ipam.AnnoIPAddressCNIRequest, ipAddress)
	// Set live migration annotation.
	kvvm.SetKVVMIAnnotation(virtv1.AllowPodBridgeNetworkLiveMigrationAnnotation, "true")
	// Set label to skip the check for PodSecurityStandards to avoid irrelevant alerts related to a privileged virtual machine pod.
	kvvm.SetKVVMILabel(annotations.SkipPodSecurityStandardsCheckLabel, "true")
	return nil
}
