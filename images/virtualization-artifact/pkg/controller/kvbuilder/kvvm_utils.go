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
	"slices"
	"strings"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/pointer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
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

func GenerateDiskName(kind v1alpha2.BlockDeviceKind, name string) string {
	switch kind {
	case v1alpha2.DiskDevice:
		return VDDiskPrefix + name
	case v1alpha2.ImageDevice:
		return VIDiskPrefix + name
	case v1alpha2.ClusterImageDevice:
		return CVIDiskPrefix + name
	}
	return ""
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

func ApplyVirtualMachineSpec(
	kvvm *KVVM, vm *v1alpha2.VirtualMachine,
	vdByName map[string]*v1alpha2.VirtualDisk,
	viByName map[string]*v1alpha2.VirtualImage,
	cviByName map[string]*v1alpha2.ClusterVirtualImage,
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

	if err := applyBlockDeviceRefs(kvvm, vm, vdByName, viByName, cviByName); err != nil {
		return err
	}

	if err := kvvm.SetProvisioning(vm.Spec.Provisioning); err != nil {
		return err
	}

	kvvm.SetOwnerRef(vm, schema.GroupVersionKind{
		Group:   v1alpha2.SchemeGroupVersion.Group,
		Version: v1alpha2.SchemeGroupVersion.Version,
		Kind:    "VirtualMachine",
	})
	kvvm.AddFinalizer(v1alpha2.FinalizerKVVMProtection)

	if ipAddress != "" {
		kvvm.SetKVVMIAnnotation(netmanager.AnnoIPAddressCNIRequest, ipAddress)
	}

	kvvm.SetKVVMIAnnotation(virtv1.AllowPodBridgeNetworkLiveMigrationAnnotation, "true")
	kvvm.SetKVVMILabel(annotations.SkipPodSecurityStandardsCheckLabel, "true")

	return setNetworksAnnotation(kvvm, networkSpec)
}

func applyBlockDeviceRefs(
	kvvm *KVVM, vm *v1alpha2.VirtualMachine,
	vdByName map[string]*v1alpha2.VirtualDisk,
	viByName map[string]*v1alpha2.VirtualImage,
	cviByName map[string]*v1alpha2.ClusterVirtualImage,
) error {
	hasExplicitBootOrder := false
	for _, bd := range vm.Spec.BlockDeviceRefs {
		if bd.BootOrder != nil {
			hasExplicitBootOrder = true
			break
		}
	}

	kvvmVolumes := kvvm.Resource.Spec.Template.Spec.Volumes
	for i, bd := range vm.Spec.BlockDeviceRefs {
		if len(kvvmVolumes) > 0 && !slices.ContainsFunc(kvvmVolumes, func(v virtv1.Volume) bool { return v.Name == GenerateDiskName(bd.Kind, bd.Name) }) {
			continue
		}

		var kvBootOrder uint
		if hasExplicitBootOrder {
			if bd.BootOrder != nil {
				kvBootOrder = uint(*bd.BootOrder)
			}
		} else {
			kvBootOrder = uint(i) + 1
		}

		if err := setBlockDeviceDisk(kvvm, bd, kvBootOrder, vdByName, viByName, cviByName); err != nil {
			return err
		}
	}

	return nil
}

func setBlockDeviceDisk(
	kvvm *KVVM, bd v1alpha2.BlockDeviceSpecRef, bootOrder uint,
	vdByName map[string]*v1alpha2.VirtualDisk,
	viByName map[string]*v1alpha2.VirtualImage,
	cviByName map[string]*v1alpha2.ClusterVirtualImage,
) error {
	switch bd.Kind {
	case v1alpha2.ImageDevice:
		vi, ok := viByName[bd.Name]
		if !ok || vi == nil {
			return fmt.Errorf("unexpected error: virtual image %q should exist in the cluster; please recreate it", bd.Name)
		}
		opts := SetDiskOptions{
			Serial:       GenerateSerialFromObject(vi),
			BootOrder:    bootOrder,
			IsHotplugged: true,
		}
		switch vi.Spec.Storage {
		case v1alpha2.StorageKubernetes, v1alpha2.StoragePersistentVolumeClaim:
			opts.PersistentVolumeClaim = pointer.GetPointer(vi.Status.Target.PersistentVolumeClaim)
			opts.IsEphemeral = true
		case v1alpha2.StorageContainerRegistry:
			opts.ContainerDisk = pointer.GetPointer(vi.Status.Target.RegistryURL)
			opts.IsCdrom = imageformat.IsISO(vi.Status.Format)
		default:
			return fmt.Errorf("unexpected storage type %q for vi %s. %w", vi.Spec.Storage, vi.Name, common.ErrUnknownType)
		}
		return kvvm.SetDisk(GenerateVIDiskName(bd.Name), opts)

	case v1alpha2.ClusterImageDevice:
		cvi, ok := cviByName[bd.Name]
		if !ok || cvi == nil {
			return fmt.Errorf("unexpected error: cluster virtual image %q should exist in the cluster; please recreate it", bd.Name)
		}
		return kvvm.SetDisk(GenerateCVIDiskName(bd.Name), SetDiskOptions{
			ContainerDisk: pointer.GetPointer(cvi.Status.Target.RegistryURL),
			IsCdrom:       imageformat.IsISO(cvi.Status.Format),
			Serial:        GenerateSerialFromObject(cvi),
			BootOrder:     bootOrder,
			IsHotplugged:  true,
		})

	case v1alpha2.DiskDevice:
		vd, ok := vdByName[bd.Name]
		if !ok || vd == nil {
			return fmt.Errorf("unexpected error: virtual disk %q should exist in the cluster; please recreate it", bd.Name)
		}
		if vd.Status.Target.PersistentVolumeClaim == "" {
			return nil
		}
		return kvvm.SetDisk(GenerateVDDiskName(bd.Name), SetDiskOptions{
			PersistentVolumeClaim: pointer.GetPointer(vd.Status.Target.PersistentVolumeClaim),
			Serial:                GenerateSerialFromObject(vd),
			BootOrder:             bootOrder,
			IsHotplugged:          true,
		})

	default:
		return fmt.Errorf("unknown block device kind %q. %w", bd.Kind, common.ErrUnknownType)
	}
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
