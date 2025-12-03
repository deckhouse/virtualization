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
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/pointer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const (
	VDDiskPrefix  = "vd-"
	VIDiskPrefix  = "vi-"
	CVIDiskPrefix = "cvi-"
)

func GenerateVDDiskName(name string) string {
	return VDDiskPrefix + name
}

func GenerateVIDiskName(name string) string {
	return VIDiskPrefix + name
}

func GenerateCVIDiskName(name string) string {
	return CVIDiskPrefix + name
}

func GetOriginalDiskName(prefixedName string) (string, v1alpha2.BlockDeviceKind) {
	switch {
	case strings.HasPrefix(prefixedName, VDDiskPrefix):
		return strings.TrimPrefix(prefixedName, VDDiskPrefix), v1alpha2.DiskDevice
	case strings.HasPrefix(prefixedName, VIDiskPrefix):
		return strings.TrimPrefix(prefixedName, VIDiskPrefix), v1alpha2.ImageDevice
	case strings.HasPrefix(prefixedName, CVIDiskPrefix):
		return strings.TrimPrefix(prefixedName, CVIDiskPrefix), v1alpha2.ClusterImageDevice
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
	Image          string
	DataVolumeName string
}

func ApplyVirtualMachineSpec(
	kvvm *KVVM, vm *v1alpha2.VirtualMachine,
	vdByName map[string]*v1alpha2.VirtualDisk,
	viByName map[string]*v1alpha2.VirtualImage,
	cviByName map[string]*v1alpha2.ClusterVirtualImage,
	vmbdas map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment,
	class *v1alpha2.VirtualMachineClass,
	ipAddress string,
	networkSpec network.InterfaceSpecList,
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
	setNetwork(kvvm, networkSpec)
	kvvm.SetTablet("default-0")
	kvvm.SetNodeSelector(vm.Spec.NodeSelector, class.Spec.NodeSelector.MatchLabels)
	kvvm.SetTolerations(vm.Spec.Tolerations, class.Spec.Tolerations)
	kvvm.SetAffinity(v1alpha2.NewAffinityFromVMAffinity(vm.Spec.Affinity), class.Spec.NodeSelector.MatchExpressions)
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

		if volume.ContainerDisk != nil && volume.ContainerDisk.Hotpluggable {
			hotpluggedDevices = append(hotpluggedDevices, HotPlugDeviceSettings{
				VolumeName: volume.Name,
				Image:      volume.ContainerDisk.Image,
			})
		}
	}

	kvvm.ClearDisks()
	bootOrder := uint(1)
	for _, bd := range vm.Spec.BlockDeviceRefs {
		// bootOrder starts from 1.
		switch bd.Kind {
		case v1alpha2.ImageDevice:
			// Attach ephemeral disk for storage: Kubernetes.
			// Attach containerDisk for storage: ContainerRegistry (i.e. image from DVCR).

			vi, ok := viByName[bd.Name]
			if !ok || vi == nil {
				return fmt.Errorf("unexpected error: virtual image %q should exist in the cluster; please recreate it", bd.Name)
			}

			name := GenerateVIDiskName(bd.Name)
			switch vi.Spec.Storage {
			case v1alpha2.StorageKubernetes,
				v1alpha2.StoragePersistentVolumeClaim:
				// Attach PVC as ephemeral volume: its data will be restored to initial state on VM restart.
				if err := kvvm.SetDisk(name, SetDiskOptions{
					PersistentVolumeClaim: pointer.GetPointer(vi.Status.Target.PersistentVolumeClaim),
					IsEphemeral:           true,
					Serial:                GenerateSerialFromObject(vi),
					BootOrder:             bootOrder,
				}); err != nil {
					return err
				}
			case v1alpha2.StorageContainerRegistry:
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

		case v1alpha2.ClusterImageDevice:
			// ClusterVirtualImage is attached as containerDisk.

			cvi, ok := cviByName[bd.Name]
			if !ok || cvi == nil {
				return fmt.Errorf("unexpected error: cluster virtual image %q should exist in the cluster; please recreate it", bd.Name)
			}

			name := GenerateCVIDiskName(bd.Name)
			if err := kvvm.SetDisk(name, SetDiskOptions{
				ContainerDisk: pointer.GetPointer(cvi.Status.Target.RegistryURL),
				IsCdrom:       imageformat.IsISO(cvi.Status.Format),
				Serial:        GenerateSerialFromObject(cvi),
				BootOrder:     bootOrder,
			}); err != nil {
				return err
			}
			bootOrder++

		case v1alpha2.DiskDevice:
			// VirtualDisk is attached as a regular disk.

			vd, ok := vdByName[bd.Name]
			if !ok || vd == nil {
				return fmt.Errorf("unexpected error: virtual disk %q should exist in the cluster; please recreate it", bd.Name)
			}

			pvcName := vd.Status.Target.PersistentVolumeClaim
			// VirtualDisk doesn't have pvc yet: wait for pvc and reconcile again.
			if pvcName == "" {
				continue
			}

			name := GenerateVDDiskName(bd.Name)
			if err := kvvm.SetDisk(name, SetDiskOptions{
				PersistentVolumeClaim: pointer.GetPointer(pvcName),
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

	if err := kvvm.SetProvisioning(vm.Spec.Provisioning); err != nil {
		return err
	}

	for _, device := range hotpluggedDevices {
		name, kind := GetOriginalDiskName(device.VolumeName)

		var obj client.Object
		var exists bool

		switch kind {
		case v1alpha2.ImageDevice:
			obj, exists = viByName[name]
		case v1alpha2.ClusterImageDevice:
			obj, exists = cviByName[name]
		case v1alpha2.DiskDevice:
			obj, exists = vdByName[name]
		default:
			return fmt.Errorf("unknown block device kind %q. %w", kind, common.ErrUnknownType)
		}

		if !exists || obj == nil || obj.GetUID() == "" {
			continue
		}

		switch {
		case device.PVCName != "":
			if err := kvvm.SetDisk(device.VolumeName, SetDiskOptions{
				PersistentVolumeClaim: pointer.GetPointer(device.PVCName),
				IsHotplugged:          true,
				Serial:                GenerateSerialFromObject(obj),
			}); err != nil {
				return err
			}
		case device.Image != "":
			if err := kvvm.SetDisk(device.VolumeName, SetDiskOptions{
				ContainerDisk: pointer.GetPointer(device.Image),
				IsHotplugged:  true,
				Serial:        GenerateSerialFromObject(obj),
			}); err != nil {
				return err
			}
		}
	}

	kvvm.SetOwnerRef(vm, schema.GroupVersionKind{
		Group:   v1alpha2.SchemeGroupVersion.Group,
		Version: v1alpha2.SchemeGroupVersion.Version,
		Kind:    "VirtualMachine",
	})
	kvvm.AddFinalizer(v1alpha2.FinalizerKVVMProtection)

	// Set ip address cni request annotation.
	kvvm.SetKVVMIAnnotation(netmanager.AnnoIPAddressCNIRequest, ipAddress)
	// Set live migration annotation.
	kvvm.SetKVVMIAnnotation(virtv1.AllowPodBridgeNetworkLiveMigrationAnnotation, "true")
	// Set label to skip the check for PodSecurityStandards to avoid irrelevant alerts related to a privileged virtual machine pod.
	kvvm.SetKVVMILabel(annotations.SkipPodSecurityStandardsCheckLabel, "true")

	// Set annotation for request network configuration.
	err := setNetworksAnnotation(kvvm, networkSpec)
	if err != nil {
		return err
	}
	return nil
}

func ApplyMigrationVolumes(kvvm *KVVM, vm *v1alpha2.VirtualMachine, vdsByName map[string]*v1alpha2.VirtualDisk) error {
	bootOrder := uint(1)
	var updateVolumesStrategy *virtv1.UpdateVolumesStrategy = nil

	for _, bd := range vm.Status.BlockDeviceRefs {
		if bd.Kind != v1alpha2.DiskDevice {
			if !bd.Hotplugged {
				bootOrder++
			}
			continue
		}

		vd := vdsByName[bd.Name]
		if vd == nil {
			continue
		}

		var pvcName string
		migrating, _ := conditions.GetCondition(vdcondition.MigratingType, vd.Status.Conditions)
		if migrating.Status == metav1.ConditionTrue && conditions.IsLastUpdated(migrating, vd) && vd.Status.MigrationState.TargetPVC != "" {
			pvcName = vd.Status.MigrationState.TargetPVC
			updateVolumesStrategy = ptr.To(virtv1.UpdateVolumesStrategyMigration)
		}
		if pvcName == "" {
			continue
		}

		name := GenerateVDDiskName(bd.Name)
		opts := SetDiskOptions{
			PersistentVolumeClaim: pointer.GetPointer(pvcName),
			Serial:                GenerateSerialFromObject(vd),
			IsHotplugged:          bd.Hotplugged,
		}
		if !bd.Hotplugged {
			opts.BootOrder = bootOrder
			bootOrder++
		}
		if err := kvvm.SetDisk(name, opts); err != nil {
			return err
		}
	}

	kvvm.SetUpdateVolumesStrategy(updateVolumesStrategy)

	return nil
}

func setNetwork(kvvm *KVVM, networkSpec network.InterfaceSpecList) {
	kvvm.ClearNetworkInterfaces()
	kvvm.SetNetworkInterface(network.NameDefaultInterface, "")

	for _, n := range networkSpec {
		kvvm.SetNetworkInterface(n.InterfaceName, n.MAC)
	}
}

func setNetworksAnnotation(kvvm *KVVM, networkSpec network.InterfaceSpecList) error {
	networkConfig := networkSpec
	networkConfigStr, err := networkConfig.ToString()
	if err != nil {
		return err
	}
	kvvm.SetKVVMIAnnotation(annotations.AnnNetworksSpec, networkConfigStr)
	return nil
}
