package kvbuilder

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	virtv1 "kubevirt.io/api/core/v1"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

// TODO(VM): Implement at this level some mechanics supporting "effectiveSpec" logic
// TODO(VM): KVVM builder should know which fields are allowed to be changed on-fly, and what params need a new KVVM instance.
// TODO(VM): Somehow report from this layer that "restart is needed" and controller will do other "effectiveSpec"-related stuff.

const CloudInitDiskName = "cloudinit"

type KVVMOptions struct {
	EnableParavirtualization bool
	OsType                   virtv2.OsType

	// These options are for local development mode
	ForceBridgeNetworkBinding bool
	DisableHypervSyNIC        bool
}

type KVVM struct {
	helper.ResourceBuilder[*virtv1.VirtualMachine]
	opts KVVMOptions
}

func NewKVVM(currentKVVM *virtv1.VirtualMachine, opts KVVMOptions) *KVVM {
	return &KVVM{
		ResourceBuilder: helper.NewResourceBuilder(currentKVVM, helper.ResourceBuilderOptions{ResourceExists: true}),
		opts:            opts,
	}
}

func NewEmptyKVVM(name types.NamespacedName, opts KVVMOptions) *KVVM {
	return &KVVM{
		opts: opts,
		ResourceBuilder: helper.NewResourceBuilder(
			&virtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name.Name,
					Namespace: name.Namespace,
				},
				Spec: virtv1.VirtualMachineSpec{
					Template: &virtv1.VirtualMachineInstanceTemplateSpec{},
				},
			}, helper.ResourceBuilderOptions{},
		),
	}
}

func (b *KVVM) SetCPUModel(model string) {
	b.Resource.Spec.Template.Spec.Domain.CPU = &virtv1.CPU{
		Model: model,
	}
}

func (b *KVVM) SetRunPolicy(runPolicy virtv2.RunPolicy) {
	switch runPolicy {
	case virtv2.AlwaysOnPolicy:
		b.Resource.Spec.RunStrategy = util.GetPointer(virtv1.RunStrategyAlways)
	case virtv2.AlwaysOffPolicy:
		b.Resource.Spec.RunStrategy = util.GetPointer(virtv1.RunStrategyHalted)
	case virtv2.ManualPolicy:
		if !b.ResourceExists {
			// initialize only
			b.Resource.Spec.RunStrategy = util.GetPointer(virtv1.RunStrategyManual)
		}
	case virtv2.AlwaysOnUnlessStoppedManualy:
		if !b.ResourceExists {
			// initialize only
			b.Resource.Spec.RunStrategy = util.GetPointer(virtv1.RunStrategyAlways)
		}
	default:
		panic(fmt.Sprintf("unexpected runPolicy %q", runPolicy))
	}
}

func (b *KVVM) SetNodeSelector(nodeSelector map[string]string) {
	b.Resource.Spec.Template.Spec.NodeSelector = nodeSelector
}

func (b *KVVM) SetTolerations(tolerations []corev1.Toleration) {
	b.Resource.Spec.Template.Spec.Tolerations = tolerations
}

func (b *KVVM) SetPriorityClassName(priorityClassName string) {
	b.Resource.Spec.Template.Spec.PriorityClassName = priorityClassName
}

func (b *KVVM) SetAffinity(affinity *corev1.Affinity) {
	b.Resource.Spec.Template.Spec.Affinity = affinity
}

func (b *KVVM) SetTerminationGracePeriod(period *int64) {
	b.Resource.Spec.Template.Spec.TerminationGracePeriodSeconds = period
}

func (b *KVVM) SetTopologySpreadConstraint(topology []corev1.TopologySpreadConstraint) {
	b.Resource.Spec.Template.Spec.TopologySpreadConstraints = topology
}

func (b *KVVM) SetResourceRequirements(cores int, coreFraction, memorySize string) {
	b.Resource.Spec.Template.Spec.Domain.Resources = virtv1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    *b.mustGetCPURequest(cores, coreFraction),
			corev1.ResourceMemory: resource.MustParse(memorySize),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    *b.getCPULimit(cores),
			corev1.ResourceMemory: resource.MustParse(memorySize),
		},
	}
}

func (b *KVVM) mustGetCPURequest(cores int, coreFraction string) *resource.Quantity {
	if coreFraction == "" {
		return b.getCPULimit(cores)
	}
	fraction := intstr.FromString(coreFraction)
	req, err := intstr.GetScaledValueFromIntOrPercent(&fraction, cores*1000, true)
	if err != nil {
		panic(fmt.Errorf("failed to calculate coreFraction. %w", err))
	}
	if req == 0 {
		return b.getCPULimit(cores)
	}
	return resource.NewMilliQuantity(int64(req), resource.DecimalSI)
}

func (b *KVVM) getCPULimit(cores int) *resource.Quantity {
	return resource.NewQuantity(int64(cores), resource.DecimalSI)
}

type SetDiskOptions struct {
	Provisioning *virtv2.Provisioning

	ContainerDisk         *string
	PersistentVolumeClaim *string

	IsHotplugged bool
	IsCdrom      bool
}

func (b *KVVM) ClearDisks() {
	b.Resource.Spec.Template.Spec.Domain.Devices.Disks = nil
	b.Resource.Spec.Template.Spec.Volumes = nil
}

func (b *KVVM) SetDisk(name string, opts SetDiskOptions) {
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

	disk := virtv1.Disk{
		Name:       name,
		DiskDevice: dd,
	}
	b.Resource.Spec.Template.Spec.Domain.Devices.Disks = util.SetArrayElem(
		b.Resource.Spec.Template.Spec.Domain.Devices.Disks, disk,
		func(v1, v2 virtv1.Disk) bool {
			return v1.Name == v2.Name
		}, true,
	)

	var vs virtv1.VolumeSource
	switch {
	case opts.PersistentVolumeClaim != nil:
		vs.PersistentVolumeClaim = &virtv1.PersistentVolumeClaimVolumeSource{
			PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: *opts.PersistentVolumeClaim,
			},
			Hotpluggable: opts.IsHotplugged,
		}
	case opts.ContainerDisk != nil:
		vs.ContainerDisk = &virtv1.ContainerDiskSource{
			Image: *opts.ContainerDisk,
		}

	case opts.Provisioning != nil:
		switch opts.Provisioning.Type {
		case virtv2.ProvisioningTypeUserData:
			vs.CloudInitNoCloud = &virtv1.CloudInitNoCloudSource{
				UserData: opts.Provisioning.UserData,
			}
		case virtv2.ProvisioningTypeUserDataSecret:
			vs.CloudInitNoCloud = &virtv1.CloudInitNoCloudSource{
				UserDataSecretRef: opts.Provisioning.UserDataSecretRef,
			}
		default:
			panic("expected either Provisioning.UserData or Provisioning.UserDataSecretRef to be set, please report a bug")
		}

	default:
		panic("expected either opts.PersistentVolumeClaim or opts.ContainerDisk to be set, please report a bug")
	}

	volume := virtv1.Volume{
		Name:         name,
		VolumeSource: vs,
	}
	b.Resource.Spec.Template.Spec.Volumes = util.SetArrayElem(
		b.Resource.Spec.Template.Spec.Volumes, volume,
		func(v1, v2 virtv1.Volume) bool {
			return v1.Name == v2.Name
		}, true,
	)
}

func (b *KVVM) SetTablet(name string) {
	i := virtv1.Input{
		Name: name,
		Bus:  virtv1.InputBusUSB,
		Type: virtv1.InputTypeTablet,
	}

	b.Resource.Spec.Template.Spec.Domain.Devices.Inputs = util.SetArrayElem(
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

func (b *KVVM) SetCloudInit(p *virtv2.Provisioning) {
	if p != nil {
		b.SetDisk(CloudInitDiskName, SetDiskOptions{Provisioning: p})
	}
}

func (b KVVM) GetCloudInitSettings() map[string]interface{} {
	var disk virtv1.Disk
	for _, d := range b.Resource.Spec.Template.Spec.Domain.Devices.Disks {
		if d.Name == CloudInitDiskName {
			disk = d
			break
		}
	}
	var volume virtv1.Volume
	for _, v := range b.Resource.Spec.Template.Spec.Volumes {
		if v.Name == CloudInitDiskName && v.CloudInitNoCloud != nil {
			volume = v
			break
		}
	}
	return map[string]interface{}{
		"disk":   &disk,
		"volume": &volume,
	}
}

func (b *KVVM) SetOsType(osType virtv2.OsType) {
	switch osType {
	case virtv2.Windows:
		b.Resource.Spec.Template.Spec.Domain.Machine = &virtv1.Machine{
			Type: "q35",
		}
		b.Resource.Spec.Template.Spec.Domain.Devices.AutoattachInputDevice = util.GetPointer(true)
		b.Resource.Spec.Template.Spec.Domain.Devices.TPM = &virtv1.TPMDevice{}
		b.Resource.Spec.Template.Spec.Domain.Features = &virtv1.Features{
			ACPI: virtv1.FeatureState{Enabled: util.GetPointer(true)},
			APIC: &virtv1.FeatureAPIC{Enabled: util.GetPointer(true)},
			SMM:  &virtv1.FeatureState{Enabled: util.GetPointer(true)},
			Hyperv: &virtv1.FeatureHyperv{
				Frequencies:     &virtv1.FeatureState{Enabled: util.GetPointer(true)},
				IPI:             &virtv1.FeatureState{Enabled: util.GetPointer(true)},
				Reenlightenment: &virtv1.FeatureState{Enabled: util.GetPointer(true)},
				Relaxed:         &virtv1.FeatureState{Enabled: util.GetPointer(true)},
				Reset:           &virtv1.FeatureState{Enabled: util.GetPointer(true)},
				Runtime:         &virtv1.FeatureState{Enabled: util.GetPointer(true)},
				Spinlocks: &virtv1.FeatureSpinlocks{
					Enabled: util.GetPointer(true),
					Retries: util.GetPointer[uint32](8191),
				},
				TLBFlush: &virtv1.FeatureState{Enabled: util.GetPointer(true)},
				VAPIC:    &virtv1.FeatureState{Enabled: util.GetPointer(true)},
				VPIndex:  &virtv1.FeatureState{Enabled: util.GetPointer(true)},
			},
		}

		if !b.opts.DisableHypervSyNIC {
			b.Resource.Spec.Template.Spec.Domain.Features.Hyperv.SyNIC = &virtv1.FeatureState{Enabled: util.GetPointer(true)}
			b.Resource.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer = &virtv1.SyNICTimer{
				Enabled: util.GetPointer(true),
				Direct:  &virtv1.FeatureState{Enabled: util.GetPointer(true)},
			}
		}

	case virtv2.GenericOs:
		b.Resource.Spec.Template.Spec.Domain.Machine = &virtv1.Machine{
			Type: "q35",
		}
		b.Resource.Spec.Template.Spec.Domain.Devices.AutoattachInputDevice = util.GetPointer(true)
		b.Resource.Spec.Template.Spec.Domain.Devices.Rng = &virtv1.Rng{}
		b.Resource.Spec.Template.Spec.Domain.Features = &virtv1.Features{
			ACPI: virtv1.FeatureState{Enabled: util.GetPointer(true)},
			SMM:  &virtv1.FeatureState{Enabled: util.GetPointer(true)},
		}

	case virtv2.LegacyWindows:
		panic("not implemented")
	}
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

func (b *KVVM) SetNetworkInterface(name string) {
	devPreset := DeviceOptionsPresets.Find(b.opts.EnableParavirtualization)

	net := virtv1.Network{
		Name: name,
		NetworkSource: virtv1.NetworkSource{
			Pod: &virtv1.PodNetwork{},
		},
	}
	b.Resource.Spec.Template.Spec.Networks = util.SetArrayElem(
		b.Resource.Spec.Template.Spec.Networks, net,
		func(v1, v2 virtv1.Network) bool {
			return v1.Name == v2.Name
		}, true,
	)

	i := virtv1.Interface{
		Name:  name,
		Model: devPreset.InterfaceModel,
	}
	if b.opts.ForceBridgeNetworkBinding {
		i.InterfaceBindingMethod.Bridge = &virtv1.InterfaceBridge{}
	} else {
		i.InterfaceBindingMethod.Macvtap = &virtv1.InterfaceMacvtap{}
	}
	b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces = util.SetArrayElem(
		b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces, i,
		func(v1, v2 virtv1.Interface) bool {
			return v1.Name == v2.Name
		}, true,
	)
}

func (b *KVVM) SetBootloader(bootloader virtv2.BootloaderType) {
	if b.Resource.Spec.Template.Spec.Domain.Firmware == nil {
		b.Resource.Spec.Template.Spec.Domain.Firmware = &virtv1.Firmware{}
	}

	switch bootloader {
	case "", virtv2.BIOS:
		b.Resource.Spec.Template.Spec.Domain.Firmware.Bootloader = nil
	case virtv2.EFI:
		b.Resource.Spec.Template.Spec.Domain.Firmware.Bootloader = &virtv1.Bootloader{
			EFI: &virtv1.EFI{
				SecureBoot: util.GetPointer(false),
			},
		}
	case virtv2.EFIWithSecureBoot:
		if b.Resource.Spec.Template.Spec.Domain.Features == nil {
			b.Resource.Spec.Template.Spec.Domain.Features = &virtv1.Features{}
		}
		b.Resource.Spec.Template.Spec.Domain.Features.SMM = &virtv1.FeatureState{
			Enabled: util.GetPointer(true),
		}
		b.Resource.Spec.Template.Spec.Domain.Firmware.Bootloader = &virtv1.Bootloader{
			EFI: &virtv1.EFI{SecureBoot: util.GetPointer(true)},
		}
	default:
		panic(fmt.Sprintf("unknown bootloader type %q, please report a bug", bootloader))
	}
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
