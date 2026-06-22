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
	"hash/fnv"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	VDDiskPrefix  = "vd-"
	VIDiskPrefix  = "vi-"
	CVIDiskPrefix = "cvi-"
)

// Budgets for the user-controlled part of a derived KubeVirt volume/disk name.
//
// A KubeVirt volume/disk name must be a valid DNS-1123 label (<=63), because it
// can become a container name. For containerDisks (VirtualImage/ClusterVirtualImage)
// KubeVirt additionally wraps the volume name as "volume<volumeName>-init", so the
// effective budget is tighter. These values keep the final container name within 63:
//
//	VD : "vd-"+X                       , X <= 60
//	VI : "volume"+"vi-"+X+"-init"  <=63 , X <= 49
//	CVI: "volume"+"cvi-"+X+"-init" <=63 , X <= 48
const (
	vdNameBudget  = 60
	viNameBudget  = 49
	cviNameBudget = 48
)

func GenerateVDDiskName(name string) string {
	return VDDiskPrefix + shortenDiskName(name, vdNameBudget)
}

func GenerateVIDiskName(name string) string {
	return VIDiskPrefix + shortenDiskName(name, viNameBudget)
}

func GenerateCVIDiskName(name string) string {
	return CVIDiskPrefix + shortenDiskName(name, cviNameBudget)
}

func GenerateDiskName(kind v1alpha2.BlockDeviceKind, name string) string {
	switch kind {
	case v1alpha2.DiskDevice:
		return GenerateVDDiskName(name)
	case v1alpha2.ImageDevice:
		return GenerateVIDiskName(name)
	case v1alpha2.ClusterImageDevice:
		return GenerateCVIDiskName(name)
	}
	return ""
}

// GenerateVMBDADiskName returns the derived KubeVirt volume/disk name for a VMBDA
// block device reference.
func GenerateVMBDADiskName(ref v1alpha2.VMBDAObjectRef) string {
	switch ref.Kind {
	case v1alpha2.VMBDAObjectRefKindVirtualDisk:
		return GenerateVDDiskName(ref.Name)
	case v1alpha2.VMBDAObjectRefKindVirtualImage:
		return GenerateVIDiskName(ref.Name)
	case v1alpha2.VMBDAObjectRefKindClusterVirtualImage:
		return GenerateCVIDiskName(ref.Name)
	}
	return ""
}

type nameKind struct {
	name string
	kind v1alpha2.BlockDeviceKind
}

// VolumeNameResolver reverses a derived KubeVirt volume/disk name back to the user
// resource it was generated from. Shortened/hashed names are not reversible by
// prefix-strip, so callers seed the resolver with the resources they already know
// (VM spec refs, VMBDA refs); the resolver matches by forward-generating each
// candidate's volume name. Names without a matching candidate fall back to
// prefix-strip, which stays correct for legacy names that were never shortened.
type VolumeNameResolver struct {
	byVolumeName map[string]nameKind
}

func NewVolumeNameResolver() *VolumeNameResolver {
	return &VolumeNameResolver{byVolumeName: make(map[string]nameKind)}
}

// Add registers a candidate user resource by its kind and name.
func (r *VolumeNameResolver) Add(kind v1alpha2.BlockDeviceKind, name string) {
	if volumeName := GenerateDiskName(kind, name); volumeName != "" {
		r.byVolumeName[volumeName] = nameKind{name: name, kind: kind}
	}
}

// Resolve returns the user resource (name, kind) for a derived volume name, or
// ("", "") for volumes that are not vd/vi/cvi block devices. It matches the
// seeded candidates first and falls back to prefix-strip for legacy short names.
func (r *VolumeNameResolver) Resolve(volumeName string) (string, v1alpha2.BlockDeviceKind) {
	if nk, ok := r.byVolumeName[volumeName]; ok {
		return nk.name, nk.kind
	}
	return GetOriginalDiskName(volumeName)
}

// shortenDiskName maps a user resource name onto the user-controlled part of a
// KubeVirt volume/disk name within the given budget.
//
// If the name already fits the budget and is a valid DNS-1123 label, it is
// returned unchanged (passthrough) — this keeps derived names byte-identical for
// every name allowed by the previous validation, so existing KubeVirt volumes are
// never renamed. Otherwise the name is deterministically shortened to a readable
// prefix plus an FNV-1a 64-bit hash of the full name, keeping the result a valid
// label within the budget. The hash is taken from the full name so that distinct
// names never share a derived name due to truncation or sanitization.
func shortenDiskName(name string, budget int) string {
	if len(name) <= budget && len(kvalidation.IsDNS1123Label(name)) == 0 {
		return name
	}

	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	suffix := hex.EncodeToString(h.Sum(nil)) // 16 hex chars for a 64-bit hash

	readable := strings.Trim(sanitizeLabel(name), "-")
	if maxReadable := budget - 1 - len(suffix); len(readable) > maxReadable {
		readable = strings.TrimRight(readable[:maxReadable], "-")
	}
	if readable == "" {
		return suffix
	}
	return readable + "-" + suffix
}

// sanitizeLabel replaces every character that is not allowed in a DNS-1123 label
// with '-'. Resource names are already lowercase DNS subdomains, so in practice
// this only rewrites '.'.
func sanitizeLabel(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
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
	vmbdaByBlockDeviceRef map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment,
	class *v1alpha2.VirtualMachineClass,
	ipAddress string,
	networkSpec network.InterfaceSpecList,
	isVmRunning bool,
) error {
	if err := kvvm.SetRunPolicy(vm.Spec.RunPolicy); err != nil {
		return err
	}
	if err := kvvm.SetOSType(vm.Spec.OsType); err != nil {
		return err
	}
	if err := kvvm.SetBootloader(vm.Spec.Bootloader); err != nil {
		return err
	}
	if err := kvvm.SetCPUModel(class); err != nil {
		return err
	}

	kvvm.SetUSBMigrationStrategy()
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

	if err := applyBlockDeviceRefs(kvvm, vm, isVmRunning, vdByName, viByName, cviByName, vmbdaByBlockDeviceRef); err != nil {
		return err
	}

	kvvm.SetGPU(vm.Name, vm.Annotations[annotations.AnnVMGPUID])

	if err := kvvm.SetProvisioning(vm.Spec.Provisioning); err != nil {
		return err
	}

	kvvm.SetOwnerRef(vm, schema.GroupVersionKind{
		Group:   v1alpha2.SchemeGroupVersion.Group,
		Version: v1alpha2.SchemeGroupVersion.Version,
		Kind:    "VirtualMachine",
	})

	if ipAddress != "" {
		// Set ip address cni request annotation.
		kvvm.SetKVVMIAnnotation(netmanager.AnnoIPAddressCNIRequest, ipAddress)
	} else {
		kvvm.RemoveKVVMIAnnotation(netmanager.AnnoIPAddressCNIRequest)
	}

	// Set live migration annotation.
	kvvm.SetKVVMIAnnotation(virtv1.AllowPodBridgeNetworkLiveMigrationAnnotation, "true")
	// Set label to skip the check for PodSecurityStandards to avoid irrelevant alerts related to a privileged virtual machine pod.
	kvvm.SetKVVMILabel(annotations.SkipPodSecurityStandardsCheckLabel, "true")

	// Set annotation for request network configuration.
	return setNetworksAnnotation(kvvm, networkSpec)
}

func applyBlockDeviceRefs(
	kvvm *KVVM, vm *v1alpha2.VirtualMachine, isVmRunning bool,
	vdByName map[string]*v1alpha2.VirtualDisk,
	viByName map[string]*v1alpha2.VirtualImage,
	cviByName map[string]*v1alpha2.ClusterVirtualImage,
	vmbdaByBlockDeviceRef map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment,
) error {
	// Backstop against a derived-name collision. The derivation is collision-
	// resistant (64-bit hash), so this is astronomically unlikely, but SetDisk
	// replaces an existing disk/volume by name, so a silent collision would drop
	// one disk and mount another's PVC. Fail loudly instead of corrupting the VM.
	if err := detectDiskNameCollisions(vm, vmbdaByBlockDeviceRef); err != nil {
		return err
	}

	isParavirtualizationEnabled := vm.Spec.IsParavirtualizationEnabled()

	hasExplicitBootOrder := false
	for _, bd := range vm.Spec.BlockDeviceRefs {
		if bd.BootOrder != nil {
			hasExplicitBootOrder = true
			break
		}
	}

	specDiskNames := make(map[string]struct{}, len(vm.Spec.BlockDeviceRefs))
	for _, bd := range vm.Spec.BlockDeviceRefs {
		specDiskNames[GenerateDiskName(bd.Kind, bd.Name)] = struct{}{}
	}
	hotpluggableVolumes := make(map[string]struct{}, len(kvvm.Resource.Spec.Template.Spec.Volumes))
	for _, v := range kvvm.Resource.Spec.Template.Spec.Volumes {
		if v.ContainerDisk != nil && v.ContainerDisk.Hotpluggable || v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.Hotpluggable {
			hotpluggableVolumes[v.Name] = struct{}{}
		}
	}
	vmbdaDiskNames := make(map[string]struct{}, len(vmbdaByBlockDeviceRef))
	for ref := range vmbdaByBlockDeviceRef {
		diskName := GenerateDiskName(v1alpha2.BlockDeviceKind(ref.Kind), ref.Name)
		if diskName != "" {
			vmbdaDiskNames[diskName] = struct{}{}
		}
	}

	// This is needed to support disk removal in old VMs with static disks
	cleanupRemovedStaticDisks(kvvm, specDiskNames, hotpluggableVolumes, vmbdaDiskNames, isVmRunning)

	kvvmVolumes := kvvm.Resource.Spec.Template.Spec.Volumes
	for i, bd := range vm.Spec.BlockDeviceRefs {
		diskName := GenerateDiskName(bd.Kind, bd.Name)
		// When VM is stopped, update disks unconditionally.
		if isVmRunning && isParavirtualizationEnabled && len(kvvmVolumes) > 0 && !slices.ContainsFunc(kvvmVolumes, func(v virtv1.Volume) bool { return v.Name == diskName }) {
			continue
		}

		var kvBootOrder uint
		if hasExplicitBootOrder {
			if bd.BootOrder != nil {
				kvBootOrder = *bd.BootOrder
			}
		} else {
			kvBootOrder = uint(i) + 1
		}

		_, hotpluggable := hotpluggableVolumes[diskName]
		if err := setBlockDeviceDisk(kvvm, bd, kvBootOrder, hotpluggable || (isParavirtualizationEnabled && !isVmRunning), vdByName, viByName, cviByName); err != nil {
			return err
		}
	}

	if err := syncAttachedVMBDAHotplugVolumes(kvvm, vdByName, viByName, cviByName, vmbdaByBlockDeviceRef); err != nil {
		return err
	}

	return nil
}

// detectDiskNameCollisions returns an error if two distinct block devices of this
// VM (spec refs or VMBDA-attached) derive the same KubeVirt volume/disk name.
func detectDiskNameCollisions(vm *v1alpha2.VirtualMachine, vmbdaByBlockDeviceRef map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment) error {
	owners := make(map[string]nameKind)
	check := func(kind v1alpha2.BlockDeviceKind, name string) error {
		diskName := GenerateDiskName(kind, name)
		if diskName == "" {
			return nil
		}
		cur := nameKind{name: name, kind: kind}
		if prev, ok := owners[diskName]; ok && prev != cur {
			return fmt.Errorf("%s %q and %s %q resolve to the same internal disk name and cannot be attached to the same virtual machine; recreate one of them with a different name",
				prev.kind, prev.name, kind, name)
		}
		owners[diskName] = cur
		return nil
	}

	for _, bd := range vm.Spec.BlockDeviceRefs {
		if err := check(bd.Kind, bd.Name); err != nil {
			return err
		}
	}
	for ref := range vmbdaByBlockDeviceRef {
		if err := check(v1alpha2.BlockDeviceKind(ref.Kind), ref.Name); err != nil {
			return err
		}
	}
	return nil
}

func cleanupRemovedStaticDisks(kvvm *KVVM, specDiskNames, hotpluggableVolumes, vmbdaDiskNames map[string]struct{}, isVmRunning bool) {
	// Disks attached via VMBDA should not be removed from KVVM spec even when VM is stopped.
	// If VMBDA exists, the disk is associated with this VM - VMBDA controller will
	// handle cleanup when VMBDA is deleted.
	isRemovedStatic := func(name string) bool {
		_, kind := GetOriginalDiskName(name)
		_, inSpec := specDiskNames[name]
		_, hotpluggable := hotpluggableVolumes[name]
		_, attachedViaVMBDA := vmbdaDiskNames[name]

		// Don't remove disks that are attached via VMBDA.
		if attachedViaVMBDA {
			return false
		}
		// When VM is stopped, remove all disks that are not in VM spec.
		if !isVmRunning {
			return kind != "" && !inSpec
		}
		return kind != "" && !inSpec && !hotpluggable
	}

	kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks = slices.DeleteFunc(
		kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks,
		func(d virtv1.Disk) bool { return isRemovedStatic(d.Name) },
	)
	kvvm.Resource.Spec.Template.Spec.Volumes = slices.DeleteFunc(
		kvvm.Resource.Spec.Template.Spec.Volumes,
		func(v virtv1.Volume) bool { return isRemovedStatic(v.Name) },
	)
}

func syncAttachedVMBDAHotplugVolumes(
	kvvm *KVVM,
	vdByName map[string]*v1alpha2.VirtualDisk,
	viByName map[string]*v1alpha2.VirtualImage,
	cviByName map[string]*v1alpha2.ClusterVirtualImage,
	vmbdaByBlockDeviceRef map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment,
) error {
	kvvmVolumes := kvvm.Resource.Spec.Template.Spec.Volumes

	for ref := range vmbdaByBlockDeviceRef {
		diskName := GenerateDiskName(v1alpha2.BlockDeviceKind(ref.Kind), ref.Name)
		if diskName == "" {
			continue
		}

		if !slices.ContainsFunc(kvvmVolumes, func(v virtv1.Volume) bool { return v.Name == diskName }) {
			continue
		}

		if vd, ok := vdByName[ref.Name]; ok && vd != nil {
			if isVolumeMigrating(vd) {
				continue
			}
		}

		if err := setVMBDABlockDeviceDisk(kvvm, ref, vdByName, viByName, cviByName); err != nil {
			return err
		}
	}

	return nil
}

func setBlockDeviceDisk(
	kvvm *KVVM, bd v1alpha2.BlockDeviceSpecRef, bootOrder uint, hotpluggable bool,
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
			IsHotplugged: hotpluggable,
		}
		opts.IsCdrom = imageformat.IsISO(vi.Status.Format)
		switch vi.Spec.Storage {
		case v1alpha2.StorageKubernetes, v1alpha2.StoragePersistentVolumeClaim:
			opts.PersistentVolumeClaim = ptr.To(vi.Status.Target.PersistentVolumeClaim)
		case v1alpha2.StorageContainerRegistry:
			opts.ContainerDisk = ptr.To(vi.Status.Target.RegistryURL)
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
			ContainerDisk: ptr.To(cvi.Status.Target.RegistryURL),
			IsCdrom:       imageformat.IsISO(cvi.Status.Format),
			Serial:        GenerateSerialFromObject(cvi),
			BootOrder:     bootOrder,
			IsHotplugged:  hotpluggable,
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
			PersistentVolumeClaim: ptr.To(vd.Status.Target.PersistentVolumeClaim),
			Serial:                GenerateSerialFromObject(vd),
			BootOrder:             bootOrder,
			IsHotplugged:          hotpluggable,
		})

	default:
		return fmt.Errorf("unknown block device kind %q. %w", bd.Kind, common.ErrUnknownType)
	}
}

func setVMBDABlockDeviceDisk(
	kvvm *KVVM,
	ref v1alpha2.VMBDAObjectRef,
	vdByName map[string]*v1alpha2.VirtualDisk,
	viByName map[string]*v1alpha2.VirtualImage,
	cviByName map[string]*v1alpha2.ClusterVirtualImage,
) error {
	switch ref.Kind {
	case v1alpha2.VMBDAObjectRefKindVirtualDisk:
		name := GenerateVDDiskName(ref.Name)
		vd, ok := vdByName[ref.Name]
		if !ok || vd == nil {
			removeDisk(kvvm, name)
			return nil
		}

		if vd.Status.Phase == v1alpha2.DiskTerminating || vd.Status.Target.PersistentVolumeClaim == "" {
			removeDisk(kvvm, name)
			return nil
		}

		return kvvm.SetDisk(name, SetDiskOptions{
			PersistentVolumeClaim: ptr.To(vd.Status.Target.PersistentVolumeClaim),
			Serial:                GenerateSerialFromObject(vd),
			IsHotplugged:          true,
		})
	case v1alpha2.VMBDAObjectRefKindVirtualImage:
		// The image ref may not be reflected in block device refs yet during the hotplug
		// attach window: the volume is already present in KVVM, disk options will be set on a
		// later reconcile. Skip instead of aborting the whole reconcile (do not removeDisk —
		// an attached image is finalizer-protected, so a missing map entry means the status
		// lagged, not that the image is gone). Mirrors the tolerant VirtualDisk branch above.
		if vi, ok := viByName[ref.Name]; !ok || vi == nil {
			return nil
		}
		return setBlockDeviceDisk(kvvm, v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.ImageDevice, Name: ref.Name}, 0, true, vdByName, viByName, cviByName)
	case v1alpha2.VMBDAObjectRefKindClusterVirtualImage:
		if cvi, ok := cviByName[ref.Name]; !ok || cvi == nil {
			return nil
		}
		return setBlockDeviceDisk(kvvm, v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.ClusterImageDevice, Name: ref.Name}, 0, true, vdByName, viByName, cviByName)
	default:
		return fmt.Errorf("unknown VMBDA block device kind %q. %w", ref.Kind, common.ErrUnknownType)
	}
}

func removeDisk(kvvm *KVVM, name string) {
	kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks = slices.DeleteFunc(
		kvvm.Resource.Spec.Template.Spec.Domain.Devices.Disks,
		func(d virtv1.Disk) bool { return d.Name == name },
	)
	kvvm.Resource.Spec.Template.Spec.Volumes = slices.DeleteFunc(
		kvvm.Resource.Spec.Template.Spec.Volumes,
		func(v virtv1.Volume) bool { return v.Name == name },
	)
}

func isVolumeMigrating(vd *v1alpha2.VirtualDisk) bool {
	return !vd.Status.MigrationState.StartTimestamp.IsZero() && vd.Status.MigrationState.EndTimestamp.IsZero()
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
		if isVolumeMigrating(vd) && vd.Status.MigrationState.TargetPVC != "" {
			pvcName = vd.Status.MigrationState.TargetPVC
			updateVolumesStrategy = ptr.To(virtv1.UpdateVolumesStrategyMigration)
		}
		if pvcName == "" {
			continue
		}

		name := GenerateVDDiskName(bd.Name)
		opts := SetDiskOptions{
			PersistentVolumeClaim: ptr.To(pvcName),
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
	desiredByName := make(map[string]struct{}, len(networkSpec))
	for _, n := range networkSpec {
		desiredByName[n.InterfaceName] = struct{}{}
	}

	for _, iface := range slices.Clone(kvvm.Resource.Spec.Template.Spec.Domain.Devices.Interfaces) {
		if _, wanted := desiredByName[iface.Name]; wanted {
			continue
		}
		if iface.Name == network.NameDefaultInterface {
			kvvm.RemoveNetworkInterface(iface.Name)
			continue
		}
		kvvm.SetNetworkInterfaceAbsent(iface.Name)
	}

	for _, n := range networkSpec {
		kvvm.SetNetworkInterface(n.InterfaceName, n.MAC, n.ID)
	}

	moveDefaultNetworkToFront(kvvm)
}

func moveDefaultNetworkToFront(kvvm *KVVM) {
	spec := &kvvm.Resource.Spec.Template.Spec
	slices.SortStableFunc(spec.Domain.Devices.Interfaces, func(a, b virtv1.Interface) int {
		return defaultNameFirst(a.Name, b.Name)
	})
	slices.SortStableFunc(spec.Networks, func(a, b virtv1.Network) int {
		return defaultNameFirst(a.Name, b.Name)
	})
}

func defaultNameFirst(a, b string) int {
	switch {
	case a == network.NameDefaultInterface:
		return -1
	case b == network.NameDefaultInterface:
		return 1
	default:
		return 0
	}
}

func setNetworksAnnotation(kvvm *KVVM, networkSpec network.InterfaceSpecList) error {
	networkConfig := networkSpec
	networkConfigStr, err := networkConfig.ToString()
	if err != nil {
		return err
	}
	kvvm.SetKVVMIAnnotation(annotations.AnnNetworksSpec, networkConfigStr)
	kvvm.SetKVVMIAnnotation(annotations.AnnTapProvisionByDVPSupported, "true")
	return nil
}
