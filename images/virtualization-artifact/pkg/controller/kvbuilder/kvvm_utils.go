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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/ipam"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/util"
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

func GetOriginalDiskName(prefixedName string) (string, bool) {
	if strings.HasPrefix(prefixedName, VMDDiskPrefix) {
		return strings.TrimPrefix(prefixedName, VMDDiskPrefix), true
	}

	return prefixedName, false
}

type HotPlugDeviceSettings struct {
	VolumeName     string
	PVCName        string
	DataVolumeName string
}

func ApplyVirtualMachineSpec(
	kvvm *KVVM, vm *virtv2.VirtualMachine,
	vmdByName map[string]*virtv2.VirtualDisk,
	vmiByName map[string]*virtv2.VirtualImage,
	cvmiByName map[string]*virtv2.ClusterVirtualImage,
	dvcrSettings *dvcr.Settings,
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

	kvvm.SetNetworkInterface(NetworkInterfaceName)
	kvvm.SetTablet("default-0")
	kvvm.SetNodeSelector(vm.Spec.NodeSelector, class.Spec.NodeSelector.MatchLabels)
	kvvm.SetTolerations(vm.Spec.Tolerations)
	kvvm.SetAffinity(virtv2.NewAffinityFromVMAffinity(vm.Spec.Affinity), class.Spec.NodeSelector.MatchExpressions)
	kvvm.SetPriorityClassName(vm.Spec.PriorityClassName)
	kvvm.SetTerminationGracePeriod(vm.Spec.TerminationGracePeriodSeconds)
	kvvm.SetTopologySpreadConstraint(vm.Spec.TopologySpreadConstraints)
	if err := kvvm.SetResourceRequirements(vm.Spec.CPU.Cores, vm.Spec.CPU.CoreFraction, vm.Spec.Memory.Size); err != nil {
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
	for _, bd := range vm.Spec.BlockDeviceRefs {
		switch bd.Kind {
		case virtv2.ImageDevice:
			// Attach ephemeral disk for storage: Kubernetes.
			// Attach containerDisk for storage: ContainerRegistry (i.e. image from DVCR).

			vmi := vmiByName[bd.Name]

			name := GenerateVMIDiskName(bd.Name)
			switch vmi.Spec.Storage {
			case virtv2.StorageKubernetes:
				// Attach PVC as ephemeral volume: its data will be restored to initial state on VM restart.
				if err := kvvm.SetDisk(name, SetDiskOptions{
					PersistentVolumeClaim: util.GetPointer(vmi.Status.Target.PersistentVolumeClaim),
					IsEphemeral:           true,
					Serial:                name,
				}); err != nil {
					return err
				}
			case virtv2.StorageContainerRegistry:
				dvcrImage := dvcrSettings.RegistryImageForVMI(vmi.Name, vmi.Namespace)
				if err := kvvm.SetDisk(name, SetDiskOptions{
					ContainerDisk: util.GetPointer(dvcrImage),
					IsCdrom:       imageformat.IsISO(vmi.Status.Format),
					Serial:        name,
				}); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unexpected storage type %q for vi %s. %w", vmi.Spec.Storage, vmi.Name, common.ErrUnknownType)
			}

		case virtv2.ClusterImageDevice:
			// ClusterVirtualImage is attached as containerDisk.

			cvmi := cvmiByName[bd.Name]

			name := GenerateCVMIDiskName(bd.Name)
			dvcrImage := dvcrSettings.RegistryImageForCVMI(cvmi.Name)
			if err := kvvm.SetDisk(name, SetDiskOptions{
				ContainerDisk: util.GetPointer(dvcrImage),
				IsCdrom:       imageformat.IsISO(cvmi.Status.Format),
				Serial:        name,
			}); err != nil {
				return err
			}

		case virtv2.DiskDevice:
			// VirtualDisk is attached as regular disk.

			vmd := vmdByName[bd.Name]

			name := GenerateVMDDiskName(bd.Name)
			if err := kvvm.SetDisk(name, SetDiskOptions{
				PersistentVolumeClaim: util.GetPointer(vmd.Status.Target.PersistentVolumeClaim),
				Serial:                name,
			}); err != nil {
				return err
			}

		default:
			return fmt.Errorf("unknown block device kind %q. %w", bd.Kind, common.ErrUnknownType)
		}
	}

	for _, device := range hotpluggedDevices {
		switch {
		case device.PVCName != "":
			if err := kvvm.SetDisk(device.VolumeName, SetDiskOptions{
				PersistentVolumeClaim: util.GetPointer(device.PVCName),
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

	return nil
}
