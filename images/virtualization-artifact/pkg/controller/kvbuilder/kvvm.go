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
	"maps"
	"os"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/array"
	"github.com/deckhouse/virtualization-controller/pkg/common/resource_builder"
	"github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// TODO(VM): Implement at this level some mechanics supporting "effectiveSpec" logic
// TODO(VM): KVVM builder should know which fields are allowed to be changed on-fly, and what params need a new KVVM instance.
// TODO(VM): Somehow report from this layer that "restart is needed" and controller will do other "effectiveSpec"-related stuff.

const (
	CloudInitDiskName = "cloudinit"
	SysprepDiskName   = "sysprep"

	// GenericCPUModel specifies the base CPU model for Features and Discovery CPU model types.
	GenericCPUModel = "qemu64"

	MaxMemorySizeForHotplug      = 256 * 1024 * 1024 * 1024 // 256 Gi (safely limit to not overlap somewhat conservative 38 bit physical address space)
	EnableMemoryHotplugThreshold = 1 * 1024 * 1024 * 1024   // 1 Gi (no hotplug for VMs with less than 1Gi)
)

const (
	// VCPUTopologyDynamicCoresAnnotation annotation indicates "distributed by sockets" or "dynamic cores number" VCPU topology.
	VCPUTopologyDynamicCoresAnnotation = "internal.virtualization.deckhouse.io/vcpu-topology-dynamic-cores"

	CPUResourcesRequestsFractionAnnotation = "internal.virtualization.deckhouse.io/cpu-resources-requests-fraction"

	// CPUMaxCoresPerSocket is a maximum number of cores per socket.
	CPUMaxCoresPerSocket = 16
)

type KVVMOptions struct {
	EnableParavirtualization bool
	OsType                   v1alpha2.OsType

	// These options are for local development mode
	DisableHypervSyNIC bool
}

type KVVM struct {
	resource_builder.ResourceBuilder[*virtv1.VirtualMachine]
	opts KVVMOptions
}

func NewKVVM(currentKVVM *virtv1.VirtualMachine, opts KVVMOptions) *KVVM {
	return &KVVM{
		ResourceBuilder: resource_builder.NewResourceBuilder(currentKVVM, resource_builder.ResourceBuilderOptions{ResourceExists: true}),
		opts:            opts,
	}
}

func DefaultOptions(current *v1alpha2.VirtualMachine) KVVMOptions {
	return KVVMOptions{
		EnableParavirtualization: current.Spec.EnableParavirtualization,
		OsType:                   current.Spec.OsType,
		DisableHypervSyNIC:       os.Getenv("DISABLE_HYPERV_SYNIC") == "1",
	}
}

func NewEmptyKVVM(name types.NamespacedName, opts KVVMOptions) *KVVM {
	return &KVVM{
		opts: opts,
		ResourceBuilder: resource_builder.NewResourceBuilder(
			&virtv1.VirtualMachine{
				TypeMeta: metav1.TypeMeta{
					Kind:       virtv1.VirtualMachineGroupVersionKind.Kind,
					APIVersion: virtv1.VirtualMachineGroupVersionKind.GroupVersion().String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name.Name,
					Namespace: name.Namespace,
				},
				Spec: virtv1.VirtualMachineSpec{
					Template: &virtv1.VirtualMachineInstanceTemplateSpec{},
				},
			}, resource_builder.ResourceBuilderOptions{},
		),
	}
}

func (b *KVVM) SetKVVMILabel(labelKey, labelValue string) {
	labels := b.Resource.Spec.Template.ObjectMeta.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	labels[labelKey] = labelValue

	b.Resource.Spec.Template.ObjectMeta.SetLabels(labels)
}

func (b *KVVM) SetKVVMIAnnotation(annoKey, annoValue string) {
	anno := b.Resource.Spec.Template.ObjectMeta.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}

	anno[annoKey] = annoValue

	b.Resource.Spec.Template.ObjectMeta.SetAnnotations(anno)
}

func (b *KVVM) RemoveKVVMIAnnotation(annoKey string) {
	anno := b.Resource.Spec.Template.ObjectMeta.GetAnnotations()
	if anno == nil {
		return
	}

	delete(anno, annoKey)

	b.Resource.Spec.Template.ObjectMeta.SetAnnotations(anno)
}

func (b *KVVM) SetCPUModel(class *v1alpha2.VirtualMachineClass) error {
	if b.Resource.Spec.Template.Spec.Domain.CPU == nil {
		b.Resource.Spec.Template.Spec.Domain.CPU = &virtv1.CPU{}
	}
	cpu := b.Resource.Spec.Template.Spec.Domain.CPU

	switch class.Spec.CPU.Type {
	case v1alpha2.CPUTypeHost:
		cpu.Model = virtv1.CPUModeHostModel
	case v1alpha2.CPUTypeHostPassthrough:
		cpu.Model = virtv1.CPUModeHostPassthrough
	case v1alpha2.CPUTypeModel:
		cpu.Model = class.Spec.CPU.Model
	case v1alpha2.CPUTypeDiscovery, v1alpha2.CPUTypeFeatures:
		cpu.Model = GenericCPUModel
		l := len(class.Status.CpuFeatures.Enabled)
		features := make([]virtv1.CPUFeature, l, l+1)
		hasSvm := false
		for i, feature := range class.Status.CpuFeatures.Enabled {
			policy := "require"
			if feature == "invtsc" {
				policy = "optional"
			}
			if feature == "svm" {
				hasSvm = true
			}
			features[i] = virtv1.CPUFeature{
				Name:   feature,
				Policy: policy,
			}
		}
		if !hasSvm {
			features = append(features, virtv1.CPUFeature{Name: "svm", Policy: "optional"})
		}
		cpu.Features = features
	default:
		return fmt.Errorf("unexpected cpu type: %q", class.Spec.CPU.Type)
	}
	return nil
}

func (b *KVVM) SetRunPolicy(runPolicy v1alpha2.RunPolicy) error {
	switch runPolicy {
	case v1alpha2.AlwaysOnPolicy,
		v1alpha2.AlwaysOffPolicy,
		v1alpha2.ManualPolicy:
		b.Resource.Spec.RunStrategy = ptr.To(virtv1.RunStrategyManual)
	case v1alpha2.AlwaysOnUnlessStoppedManually:
		if !b.ResourceExists {
			// initialize only
			b.Resource.Spec.RunStrategy = ptr.To(virtv1.RunStrategyAlways)
		}
	default:
		return fmt.Errorf("unexpected runPolicy %s. %w", runPolicy, common.ErrUnknownValue)
	}
	return nil
}

func (b *KVVM) SetNodeSelector(vmNodeSelector, classNodeSelector map[string]string) {
	if len(vmNodeSelector) == 0 && len(classNodeSelector) == 0 {
		b.Resource.Spec.Template.Spec.NodeSelector = nil
		return
	}
	selector := make(map[string]string, len(vmNodeSelector)+len(classNodeSelector))
	maps.Copy(selector, vmNodeSelector)
	maps.Copy(selector, classNodeSelector)
	b.Resource.Spec.Template.Spec.NodeSelector = selector
}

func (b *KVVM) SetTolerations(vmTolerations, classTolerations []corev1.Toleration) {
	tolerations := make([]corev1.Toleration, 0, len(vmTolerations)+len(classTolerations))
	tolerations = append(tolerations, vmTolerations...)
	tolerations = append(tolerations, classTolerations...)
	if len(tolerations) == 0 {
		b.Resource.Spec.Template.Spec.Tolerations = nil
		return
	}
	b.Resource.Spec.Template.Spec.Tolerations = tolerations
}

func (b *KVVM) SetPriorityClassName(priorityClassName string) {
	b.Resource.Spec.Template.Spec.PriorityClassName = priorityClassName
}

func (b *KVVM) SetAffinity(vmAffinity *corev1.Affinity, classMatchExpressions []corev1.NodeSelectorRequirement) {
	if len(classMatchExpressions) == 0 {
		b.Resource.Spec.Template.Spec.Affinity = vmAffinity
		return
	}
	if vmAffinity == nil {
		vmAffinity = &corev1.Affinity{}
	}
	if vmAffinity.NodeAffinity == nil {
		vmAffinity.NodeAffinity = &corev1.NodeAffinity{}
	}
	if vmAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		vmAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
	}
	if vmAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms == nil {
		vmAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = []corev1.NodeSelectorTerm{}
	}
	if len(vmAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) == 0 {
		vmAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(
			vmAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms,
			corev1.NodeSelectorTerm{MatchExpressions: classMatchExpressions})
	} else {
		for i := range vmAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
			vmAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[i].MatchExpressions = append(
				vmAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[i].MatchExpressions, classMatchExpressions...)
		}
	}

	b.Resource.Spec.Template.Spec.Affinity = vmAffinity
}

func (b *KVVM) SetTerminationGracePeriod(period *int64) {
	b.Resource.Spec.Template.Spec.TerminationGracePeriodSeconds = period
}

func (b *KVVM) SetTopologySpreadConstraint(topology []corev1.TopologySpreadConstraint) {
	b.Resource.Spec.Template.Spec.TopologySpreadConstraints = topology
}

func (b *KVVM) SetCPU(cores int, coreFraction string) error {
	// Support for VMs started with cpu configuration in requests-limits.
	// TODO delete this in the future (around 3-4 more versions after enabling cpu hotplug by default).
	if b.ResourceExists && isVMRunningWithCPUResources(b.Resource) {
		return b.setCPUNonHotpluggable(cores, coreFraction)
	}
	return b.setCPUHotpluggable(cores, coreFraction)
}

// setCPUNonHotpluggable translates cpu configuration to requests and limit in KVVM.
// Note: this is a first implementation, cpu hotplug is not compatible with this strategy.
func (b *KVVM) setCPUNonHotpluggable(cores int, coreFraction string) error {
	domainSpec := &b.Resource.Spec.Template.Spec.Domain
	if domainSpec.CPU == nil {
		domainSpec.CPU = &virtv1.CPU{}
	}
	cpuRequest, err := GetCPURequest(cores, coreFraction)
	if err != nil {
		return err
	}

	cpuLimit := GetCPULimit(cores)
	if domainSpec.Resources.Requests == nil {
		domainSpec.Resources.Requests = make(map[corev1.ResourceName]resource.Quantity)
	}
	if domainSpec.Resources.Limits == nil {
		domainSpec.Resources.Limits = make(map[corev1.ResourceName]resource.Quantity)
	}
	domainSpec.Resources.Requests[corev1.ResourceCPU] = *cpuRequest
	domainSpec.Resources.Limits[corev1.ResourceCPU] = *cpuLimit

	socketsNeeded, coresNeeded := vm.CalculateCoresAndSockets(cores)

	domainSpec.CPU.Cores = uint32(coresNeeded)
	domainSpec.CPU.Sockets = uint32(socketsNeeded)
	domainSpec.CPU.MaxSockets = uint32(socketsNeeded)
	return nil
}

// setCPUHotpluggable translates cpu configuration to settings in domain.cpu field.
// This field is compatible with memory hotplug.
// Also, remove requests-limits for memory if any.
// Note: we swap cores and sockets to bypass vm-validation webhook.
func (b *KVVM) setCPUHotpluggable(cores int, coreFraction string) error {
	domainSpec := &b.Resource.Spec.Template.Spec.Domain
	if domainSpec.CPU == nil {
		domainSpec.CPU = &virtv1.CPU{}
	}

	fraction, err := GetCPUFraction(coreFraction)
	if err != nil {
		return err
	}
	b.SetKVVMIAnnotation(CPUResourcesRequestsFractionAnnotation, strconv.Itoa(fraction))

	socketsNeeded, coresPerSocketNeeded := vm.CalculateCoresAndSockets(cores)
	// Use "dynamic cores" hotplug strategy.
	// Workaround: swap cores and sockets in domainSpec to bypass vm-validator webhook.
	b.SetKVVMIAnnotation(VCPUTopologyDynamicCoresAnnotation, "")
	domainSpec.CPU.Cores = uint32(socketsNeeded)
	domainSpec.CPU.Sockets = uint32(coresPerSocketNeeded)
	domainSpec.CPU.MaxSockets = CPUMaxCoresPerSocket

	// Remove CPU limits and requests if set by previous implementation.
	res := &b.Resource.Spec.Template.Spec.Domain.Resources
	delete(res.Requests, corev1.ResourceCPU)
	delete(res.Limits, corev1.ResourceCPU)

	return nil
}

// SetMemory sets memory in kvvm.
// There are 2 possibilities to set memory:
// 1. Use domain.memory.guest field: it enabled memory hotplugging, but not set resources.limits.
// 2. Explicitly set limits and requests in domain.resources. No hotplugging in this scenario.
//
// (1) is a new approach, and (2) should be respected for Running VMs started by previous version of the controller.
func (b *KVVM) SetMemory(memorySize resource.Quantity) {
	// Support for VMs started with memory size in requests-limits.
	// TODO delete this in the future (around 3-4 more versions after enabling memory hotplug by default).
	if b.ResourceExists && shouldKeepMemoryNonHotpluggable(b.Resource) {
		b.setMemoryNonHotpluggable(memorySize)
		return
	}
	b.setMemoryHotpluggable(memorySize)
}

// setMemoryNonHotpluggable translates memory size to requests and limits in KVVM.
// Note: this is a first implementation, memory hotplug is not compatible with this strategy.
func (b *KVVM) setMemoryNonHotpluggable(memorySize resource.Quantity) {
	res := &b.Resource.Spec.Template.Spec.Domain.Resources
	if res.Requests == nil {
		res.Requests = make(map[corev1.ResourceName]resource.Quantity)
	}
	if res.Limits == nil {
		res.Limits = make(map[corev1.ResourceName]resource.Quantity)
	}
	res.Requests[corev1.ResourceMemory] = memorySize
	res.Limits[corev1.ResourceMemory] = memorySize
}

// setMemoryHotpluggable translates memory size to settings in domain.memory field.
// This field is compatible with memory hotplug.
// Also, remove requests-limits for memory if any.
func (b *KVVM) setMemoryHotpluggable(memorySize resource.Quantity) {
	domain := &b.Resource.Spec.Template.Spec.Domain

	currentMaxGuest := int64(-1)
	if domain.Memory != nil && domain.Memory.MaxGuest != nil {
		currentMaxGuest = domain.Memory.MaxGuest.Value()
	}

	domain.Memory = &virtv1.Memory{
		Guest: &memorySize,
	}

	// Set maxMemory to enable hotplug for mem size >= 1Gi.
	hotplugThreshold := resource.NewQuantity(EnableMemoryHotplugThreshold, resource.BinarySI)
	if featuregates.Default().Enabled(featuregates.HotplugMemoryWithLiveMigration) {
		if memorySize.Cmp(*hotplugThreshold) >= 0 {
			maxMemory := resource.NewQuantity(MaxMemorySizeForHotplug, resource.BinarySI)
			domain.Memory.MaxGuest = maxMemory
		}
	}
	// Set maxGuest to 0 if hotplug is disabled now (mem size < 1Gi) and maxGuest was previously set.
	// Zero value is just a flag to patch memory and remove maxGuest before updating kvvm.
	if memorySize.Cmp(*hotplugThreshold) == -1 && currentMaxGuest > 0 {
		domain.Memory.MaxGuest = resource.NewQuantity(0, resource.BinarySI)
	}

	// Remove memory limits and requests if set by previous implementation.
	res := &b.Resource.Spec.Template.Spec.Domain.Resources
	delete(res.Requests, corev1.ResourceMemory)
	delete(res.Limits, corev1.ResourceMemory)
}

func isVMRunningWithCPUResources(kvvm *virtv1.VirtualMachine) bool {
	if kvvm == nil {
		return false
	}

	if kvvm.Status.PrintableStatus != virtv1.VirtualMachineStatusRunning {
		return false
	}

	res := kvvm.Spec.Template.Spec.Domain.Resources
	_, hasCPURequests := res.Requests[corev1.ResourceCPU]
	_, hasCPULimits := res.Limits[corev1.ResourceCPU]

	return hasCPURequests && hasCPULimits
}

func shouldKeepMemoryNonHotpluggable(kvvm *virtv1.VirtualMachine) bool {
	if kvvm == nil {
		return false
	}

	if kvvm.Status.PrintableStatus == virtv1.VirtualMachineStatusRunning || kvvm.Status.PrintableStatus == virtv1.VirtualMachineStatusMigrating {
		// Running or Migrating machines with memory resources should keep as non-hotpluggable.
		// Machines without memory resources should proceed as hotpluggable.
		res := kvvm.Spec.Template.Spec.Domain.Resources
		_, hasMemoryRequests := res.Requests[corev1.ResourceMemory]
		_, hasMemoryLimits := res.Limits[corev1.ResourceMemory]

		return hasMemoryRequests && hasMemoryLimits
	}

	// Proceed as hotpluggable if machine is not Running or Migrating.
	return false
}

func GetCPUFraction(cpuFraction string) (int, error) {
	if cpuFraction == "" {
		return 100, nil
	}
	fraction := intstr.FromString(cpuFraction)
	value, _, err := getIntOrPercentValueSafely(&fraction)
	if err != nil {
		return 0, fmt.Errorf("invalid value for cpu fraction: %w", err)
	}
	return value, nil
}

func getIntOrPercentValueSafely(intOrStr *intstr.IntOrString) (int, bool, error) {
	switch intOrStr.Type {
	case intstr.Int:
		return intOrStr.IntValue(), false, nil
	case intstr.String:
		s := intOrStr.StrVal
		if !strings.HasSuffix(s, "%") {
			return 0, false, fmt.Errorf("invalid type: string is not a percentage")
		}
		s = strings.TrimSuffix(intOrStr.StrVal, "%")

		v, err := strconv.Atoi(s)
		if err != nil {
			return 0, false, fmt.Errorf("invalid value %q: %w", intOrStr.StrVal, err)
		}
		return v, true, nil
	}
	return 0, false, fmt.Errorf("invalid type: neither int nor percentage")
}

func GetCPURequest(cores int, coreFraction string) (*resource.Quantity, error) {
	if coreFraction == "" {
		return GetCPULimit(cores), nil
	}
	fraction := intstr.FromString(coreFraction)
	req, err := intstr.GetScaledValueFromIntOrPercent(&fraction, cores*1000, true)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate coreFraction. %w", err)
	}
	if req == 0 {
		return GetCPULimit(cores), nil
	}
	return resource.NewMilliQuantity(int64(req), resource.DecimalSI), nil
}

func GetCPULimit(cores int) *resource.Quantity {
	return resource.NewQuantity(int64(cores), resource.DecimalSI)
}

type SetDiskOptions struct {
	Provisioning *v1alpha2.Provisioning

	ContainerDisk         *string
	PersistentVolumeClaim *string

	IsHotplugged bool
	IsCdrom      bool
	IsEphemeral  bool

	Serial string

	BootOrder uint
}

func (b *KVVM) ClearDisks() {
	b.Resource.Spec.Template.Spec.Domain.Devices.Disks = nil
	b.Resource.Spec.Template.Spec.Volumes = nil
}

func (b *KVVM) getExistingDiskBus(name string) virtv1.DiskBus {
	for _, d := range b.Resource.Spec.Template.Spec.Domain.Devices.Disks {
		if d.Name != name {
			continue
		}
		if d.CDRom != nil {
			return d.CDRom.Bus
		}
		if d.Disk != nil {
			return d.Disk.Bus
		}
	}
	return ""
}

func (b *KVVM) SetDisk(name string, opts SetDiskOptions) error {
	devPreset := DeviceOptionsPresets.Find(b.opts.EnableParavirtualization)

	var dd virtv1.DiskDevice
	if opts.IsCdrom {
		dd.CDRom = &virtv1.CDRomTarget{
			Bus: devPreset.CdromBus,
		}
	} else {
		dd.Disk = &virtv1.DiskTarget{
			Bus: devPreset.DiskBus,
		}
	}

	if existingBus := b.getExistingDiskBus(name); existingBus != "" {
		if opts.IsCdrom {
			dd.CDRom.Bus = existingBus
		} else {
			dd.Disk.Bus = existingBus
		}
	}

	disk := virtv1.Disk{
		Name:        name,
		DiskDevice:  dd,
		Serial:      opts.Serial,
		ErrorPolicy: ptr.To(virtv1.DiskErrorPolicyReport),
	}

	if opts.BootOrder > 0 {
		disk.BootOrder = &opts.BootOrder
	}

	b.Resource.Spec.Template.Spec.Domain.Devices.Disks = array.SetArrayElem(
		b.Resource.Spec.Template.Spec.Domain.Devices.Disks, disk,
		func(v1, v2 virtv1.Disk) bool {
			return v1.Name == v2.Name
		}, true,
	)

	var vs virtv1.VolumeSource
	switch {
	case opts.PersistentVolumeClaim != nil && !opts.IsEphemeral:
		vs.PersistentVolumeClaim = &virtv1.PersistentVolumeClaimVolumeSource{
			PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: *opts.PersistentVolumeClaim,
			},
			Hotpluggable: opts.IsHotplugged,
		}

	case opts.PersistentVolumeClaim != nil && opts.IsEphemeral:
		vs.Ephemeral = &virtv1.EphemeralVolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: *opts.PersistentVolumeClaim,
				ReadOnly:  true,
			},
		}

	case opts.ContainerDisk != nil:
		vs.ContainerDisk = &virtv1.ContainerDiskSource{
			Image:        *opts.ContainerDisk,
			Hotpluggable: opts.IsHotplugged,
		}

	case opts.Provisioning != nil:
		switch opts.Provisioning.Type {
		case v1alpha2.ProvisioningTypeSysprepRef:
			if opts.Provisioning.SysprepRef == nil {
				return fmt.Errorf("nil sysprep ref: %s", opts.Provisioning.Type)
			}

			switch opts.Provisioning.SysprepRef.Kind {
			case v1alpha2.SysprepRefKindSecret:
				vs.Sysprep = &virtv1.SysprepSource{
					Secret: &corev1.LocalObjectReference{
						Name: opts.Provisioning.SysprepRef.Name,
					},
				}
			default:
				return fmt.Errorf("unexpected sysprep ref kind: %s", opts.Provisioning.SysprepRef.Kind)
			}
		case v1alpha2.ProvisioningTypeUserData:
			vs.CloudInitNoCloud = &virtv1.CloudInitNoCloudSource{
				UserData: opts.Provisioning.UserData,
			}
		case v1alpha2.ProvisioningTypeUserDataRef:
			if opts.Provisioning.UserDataRef == nil {
				return fmt.Errorf("nil user data ref: %s", opts.Provisioning.Type)
			}

			switch opts.Provisioning.UserDataRef.Kind {
			case v1alpha2.UserDataRefKindSecret:
				vs.CloudInitNoCloud = &virtv1.CloudInitNoCloudSource{
					UserDataSecretRef: &corev1.LocalObjectReference{
						Name: opts.Provisioning.UserDataRef.Name,
					},
				}
			default:
				return fmt.Errorf("unexpected user data ref kind: %s", opts.Provisioning.UserDataRef.Kind)
			}
		default:
			return fmt.Errorf("unexpected provisioning type %s. %w", opts.Provisioning.Type, common.ErrUnknownType)
		}

	default:
		return fmt.Errorf("expected either opts.PersistentVolumeClaim or opts.ContainerDisk to be set, please report a bug")
	}

	volume := virtv1.Volume{
		Name:         name,
		VolumeSource: vs,
	}
	b.Resource.Spec.Template.Spec.Volumes = array.SetArrayElem(
		b.Resource.Spec.Template.Spec.Volumes, volume,
		func(v1, v2 virtv1.Volume) bool {
			return v1.Name == v2.Name
		}, true,
	)
	return nil
}

func (b *KVVM) SetTablet(name string) {
	i := virtv1.Input{
		Name: name,
		Bus:  virtv1.InputBusUSB,
		Type: virtv1.InputTypeTablet,
	}

	b.Resource.Spec.Template.Spec.Domain.Devices.Inputs = array.SetArrayElem(
		b.Resource.Spec.Template.Spec.Domain.Devices.Inputs, i,
		func(v1, v2 virtv1.Input) bool {
			return v1.Name == v2.Name
		}, true,
	)
}

// HasTablet checks tablet presence by its name.
func (b *KVVM) HasTablet(name string) bool {
	for _, input := range b.Resource.Spec.Template.Spec.Domain.Devices.Inputs {
		if input.Name == name && input.Type == virtv1.InputTypeTablet {
			return true
		}
	}
	return false
}

func (b *KVVM) SetProvisioning(p *v1alpha2.Provisioning) error {
	if p == nil {
		return nil
	}

	switch p.Type {
	case v1alpha2.ProvisioningTypeSysprepRef:
		return b.SetDisk(SysprepDiskName, SetDiskOptions{Provisioning: p, IsCdrom: true})
	case v1alpha2.ProvisioningTypeUserData, v1alpha2.ProvisioningTypeUserDataRef:
		return b.SetDisk(CloudInitDiskName, SetDiskOptions{Provisioning: p})
	default:
		return fmt.Errorf("unexpected provisioning type %s. %w", p.Type, common.ErrUnknownType)
	}
}

func (b *KVVM) SetOSType(osType v1alpha2.OsType) error {
	switch osType {
	case v1alpha2.Windows:
		// Need for `029-use-OFVM_CODE-for-linux.patch`
		// b.SetKVVMIAnnotation(annotations.AnnOsType, string(virtv2.Windows))

		b.Resource.Spec.Template.Spec.Domain.Machine = &virtv1.Machine{
			Type: "q35",
		}
		b.Resource.Spec.Template.Spec.Domain.Devices.AutoattachInputDevice = ptr.To(true)
		b.Resource.Spec.Template.Spec.Domain.Devices.TPM = &virtv1.TPMDevice{}
		b.Resource.Spec.Template.Spec.Domain.Features = &virtv1.Features{
			ACPI: virtv1.FeatureState{Enabled: ptr.To(true)},
			APIC: &virtv1.FeatureAPIC{Enabled: ptr.To(true)},
			SMM:  &virtv1.FeatureState{Enabled: ptr.To(true)},
			Hyperv: &virtv1.FeatureHyperv{
				Frequencies:     &virtv1.FeatureState{Enabled: ptr.To(true)},
				IPI:             &virtv1.FeatureState{Enabled: ptr.To(true)},
				Reenlightenment: &virtv1.FeatureState{Enabled: ptr.To(true)},
				Relaxed:         &virtv1.FeatureState{Enabled: ptr.To(true)},
				Reset:           &virtv1.FeatureState{Enabled: ptr.To(true)},
				Runtime:         &virtv1.FeatureState{Enabled: ptr.To(true)},
				Spinlocks: &virtv1.FeatureSpinlocks{
					Enabled: ptr.To(true),
					Retries: ptr.To[uint32](8191),
				},
				TLBFlush: &virtv1.FeatureState{Enabled: ptr.To(true)},
				VAPIC:    &virtv1.FeatureState{Enabled: ptr.To(true)},
				VPIndex:  &virtv1.FeatureState{Enabled: ptr.To(true)},
			},
		}

		if !b.opts.DisableHypervSyNIC {
			b.Resource.Spec.Template.Spec.Domain.Features.Hyperv.SyNIC = &virtv1.FeatureState{Enabled: ptr.To(true)}
			b.Resource.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer = &virtv1.SyNICTimer{
				Enabled: ptr.To(true),
				Direct:  &virtv1.FeatureState{Enabled: ptr.To(true)},
			}
		}

	case v1alpha2.GenericOs:
		b.Resource.Spec.Template.Spec.Domain.Machine = &virtv1.Machine{
			Type: "q35",
		}
		b.Resource.Spec.Template.Spec.Domain.Devices.AutoattachInputDevice = ptr.To(true)
		b.Resource.Spec.Template.Spec.Domain.Devices.TPM = nil
		b.Resource.Spec.Template.Spec.Domain.Devices.Rng = &virtv1.Rng{}
		b.Resource.Spec.Template.Spec.Domain.Features = &virtv1.Features{
			ACPI: virtv1.FeatureState{Enabled: ptr.To(true)},
			SMM:  &virtv1.FeatureState{Enabled: ptr.To(true)},
		}
	default:
		return fmt.Errorf("unexpected os type %q. %w", osType, common.ErrUnknownType)
	}
	return nil
}

// GetOSSettings returns a portion of devices and features related to d8 VM osType.
func (b *KVVM) GetOSSettings() map[string]interface{} {
	return map[string]interface{}{
		"machine": b.Resource.Spec.Template.Spec.Domain.Machine,
		"devices": map[string]interface{}{
			"autoattach": b.Resource.Spec.Template.Spec.Domain.Devices.AutoattachInputDevice,
			"tpm":        b.Resource.Spec.Template.Spec.Domain.Devices.TPM,
			"rng":        b.Resource.Spec.Template.Spec.Domain.Devices.Rng,
		},
		"features": map[string]interface{}{
			"acpi":   b.Resource.Spec.Template.Spec.Domain.Features.ACPI,
			"apic":   b.Resource.Spec.Template.Spec.Domain.Features.APIC,
			"smm":    b.Resource.Spec.Template.Spec.Domain.Features.SMM,
			"hyperv": b.Resource.Spec.Template.Spec.Domain.Features.Hyperv,
		},
	}
}

func (b *KVVM) ClearNetworkInterfaces() {
	b.Resource.Spec.Template.Spec.Networks = nil
	b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces = nil
}

func (b *KVVM) SetNetworkInterface(name, macAddress string, acpiIndex int) {
	net := virtv1.Network{
		Name: name,
		NetworkSource: virtv1.NetworkSource{
			Pod: &virtv1.PodNetwork{},
		},
	}
	b.Resource.Spec.Template.Spec.Networks = array.SetArrayElem(
		b.Resource.Spec.Template.Spec.Networks, net,
		func(v1, v2 virtv1.Network) bool {
			return v1.Name == v2.Name
		}, true,
	)

	devPreset := DeviceOptionsPresets.Find(b.opts.EnableParavirtualization)

	iface := virtv1.Interface{
		Name:      name,
		Model:     devPreset.InterfaceModel,
		ACPIIndex: acpiIndex,
	}
	iface.Bridge = &virtv1.InterfaceBridge{}
	if macAddress != "" {
		iface.MacAddress = macAddress
	}

	ifaceExists := false
	for _, i := range b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces {
		if i.Name == name {
			ifaceExists = true
			break
		}
	}

	if !ifaceExists {
		b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces = append(b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces, iface)
	}
}

func (b *KVVM) SetBootloader(bootloader v1alpha2.BootloaderType) error {
	if b.Resource.Spec.Template.Spec.Domain.Firmware == nil {
		b.Resource.Spec.Template.Spec.Domain.Firmware = &virtv1.Firmware{}
	}

	switch bootloader {
	case "", v1alpha2.BIOS:
		b.Resource.Spec.Template.Spec.Domain.Firmware.Bootloader = nil
	case v1alpha2.EFI:
		b.Resource.Spec.Template.Spec.Domain.Firmware.Bootloader = &virtv1.Bootloader{
			EFI: &virtv1.EFI{
				SecureBoot: ptr.To(false),
			},
		}
	case v1alpha2.EFIWithSecureBoot:
		if b.Resource.Spec.Template.Spec.Domain.Features == nil {
			b.Resource.Spec.Template.Spec.Domain.Features = &virtv1.Features{}
		}
		b.Resource.Spec.Template.Spec.Domain.Features.SMM = &virtv1.FeatureState{
			Enabled: ptr.To(true),
		}
		b.Resource.Spec.Template.Spec.Domain.Firmware.Bootloader = &virtv1.Bootloader{
			EFI: &virtv1.EFI{
				SecureBoot: ptr.To(true),
				Persistent: ptr.To(true),
			},
		}
	default:
		return fmt.Errorf("unexpected bootloader type %q. %w", bootloader, common.ErrUnknownType)
	}
	return nil
}

// GetBootloaderSettings returns a portion of features related to d8 VM bootloader.
func (b *KVVM) GetBootloaderSettings() map[string]interface{} {
	return map[string]interface{}{
		"firmare": b.Resource.Spec.Template.Spec.Domain.Firmware,
		"features": map[string]interface{}{
			"smm": b.Resource.Spec.Template.Spec.Domain.Features.SMM,
		},
	}
}

func (b *KVVM) SetMetadata(metadata metav1.ObjectMeta) {
	if b.ResourceExists {
		// initialize only
		return
	}
	if b.Resource.Spec.Template.ObjectMeta.Labels == nil {
		b.Resource.Spec.Template.ObjectMeta.Labels = make(map[string]string, len(metadata.Labels))
	}
	if b.Resource.Spec.Template.ObjectMeta.Annotations == nil {
		b.Resource.Spec.Template.ObjectMeta.Annotations = make(map[string]string, len(metadata.Annotations))
	}
	maps.Copy(b.Resource.Spec.Template.ObjectMeta.Labels, metadata.Labels)
	maps.Copy(b.Resource.Spec.Template.ObjectMeta.Annotations, metadata.Annotations)

	b.Resource.Spec.Template.ObjectMeta.Annotations = vm.RemoveNonPropagatableAnnotations(b.Resource.Spec.Template.ObjectMeta.Annotations)
}

func (b *KVVM) SetUpdateVolumesStrategy(strategy *virtv1.UpdateVolumesStrategy) {
	b.Resource.Spec.UpdateVolumesStrategy = strategy
}

func (b *KVVM) SetUSBMigrationStrategy() {
	b.SetKVVMIAnnotation(virtv1.USBMigrationStrategyAnn, string(virtv1.USBMigrationStrategyIgnore))
}
