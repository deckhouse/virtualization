package kvbuilder

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

// TODO(VM): Implement at this level some mechanics supporting "effectiveSpec" logic
// TODO(VM): KVVM builder should know which fields are allowed to be changed on-fly, and what params need a new KVVM instance.
// TODO(VM): Somehow report from this layer that "restart is needed" and controller will do other "effectiveSpec"-related stuff.

type KVVMOptions struct {
	EnableParavirtualization bool
	OsType                   virtv2.OsType

	// This option is for local development mode
	ForceBridgeNetworkBinding bool
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

func (b *KVVM) SetResourceRequirements(cores int, coreFraction, memorySize string) {
	_ = coreFraction
	b.Resource.Spec.Template.Spec.Domain.Resources = virtv1.ResourceRequirements{
		Requests: corev1.ResourceList{
			// FIXME: support coreFraction: req = resource.Spec.CPU.Cores * coreFraction
			// FIXME: coreFraction is percents between 0 and 100, for example: "50%"
			corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", cores)),
			corev1.ResourceMemory: resource.MustParse(memorySize),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", cores)),
			corev1.ResourceMemory: resource.MustParse(memorySize),
		},
	}
}

type AddDiskOptions struct {
	ContainerDisk         *string
	PersistentVolumeClaim *string

	IsEphemeralPVC bool
	IsCdrom        bool
}

func (b *KVVM) AddDisk(name string, opts AddDiskOptions) {
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
	b.Resource.Spec.Template.Spec.Domain.Devices.Disks = append(b.Resource.Spec.Template.Spec.Domain.Devices.Disks, disk)

	var vs virtv1.VolumeSource

	switch {
	case opts.PersistentVolumeClaim != nil:
		if opts.IsEphemeralPVC {
			vs.Ephemeral = &virtv1.EphemeralVolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: *opts.PersistentVolumeClaim,
				},
			}
		} else {
			vs.PersistentVolumeClaim = &virtv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: *opts.PersistentVolumeClaim,
				},
			}
		}

	case opts.ContainerDisk != nil:
		vs.ContainerDisk = &virtv1.ContainerDiskSource{
			Image: *opts.ContainerDisk,
		}

	default:
		panic("expected either opts.PersistentVolumeClaim or opts.ContainerDisk to be set, please report a bug")
	}

	volume := virtv1.Volume{
		Name:         name,
		VolumeSource: vs,
	}
	b.Resource.Spec.Template.Spec.Volumes = append(b.Resource.Spec.Template.Spec.Volumes, volume)
}

func (b *KVVM) AddTablet(name string) {
	i := virtv1.Input{
		Name: name,
		Bus:  virtv1.InputBusUSB,
		Type: virtv1.InputTypeTablet,
	}
	b.Resource.Spec.Template.Spec.Domain.Devices.Inputs = append(b.Resource.Spec.Template.Spec.Domain.Devices.Inputs, i)
}

func (b *KVVM) AddCloudInit() {
	// TODO(VM): implement this helper to attach cloud-init volume with an initialization data
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
				SyNIC: &virtv1.FeatureState{Enabled: util.GetPointer(true)},
				SyNICTimer: &virtv1.SyNICTimer{
					Enabled: util.GetPointer(true),
					Direct:  &virtv1.FeatureState{Enabled: util.GetPointer(true)},
				},
				TLBFlush: &virtv1.FeatureState{Enabled: util.GetPointer(true)},
				VAPIC:    &virtv1.FeatureState{Enabled: util.GetPointer(true)},
				VPIndex:  &virtv1.FeatureState{Enabled: util.GetPointer(true)},
			},
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

func (b *KVVM) AddNetworkInterface(name string) {
	devPreset := DeviceOptionsPresets.Find(b.opts.EnableParavirtualization)

	foundNetwork := false
	for _, n := range b.Resource.Spec.Template.Spec.Networks {
		if n.Name == name {
			foundNetwork = true
			break
		}
	}
	if !foundNetwork {
		b.Resource.Spec.Template.Spec.Networks = append(b.Resource.Spec.Template.Spec.Networks, virtv1.Network{
			Name: name,
			NetworkSource: virtv1.NetworkSource{
				Pod: &virtv1.PodNetwork{},
			},
		})
	}

	i := virtv1.Interface{
		Name:  name,
		Model: devPreset.InterfaceModel,
	}

	if b.opts.ForceBridgeNetworkBinding {
		i.InterfaceBindingMethod.Bridge = &virtv1.InterfaceBridge{}
	} else {
		i.InterfaceBindingMethod.Macvtap = &virtv1.InterfaceMacvtap{}
	}

	b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces = append(b.Resource.Spec.Template.Spec.Domain.Devices.Interfaces, i)
}

func (b *KVVM) SetBootloader() {
	// TODO(VM): implement bootloader param switch (BIOS used by default in KubeVirt)
}
