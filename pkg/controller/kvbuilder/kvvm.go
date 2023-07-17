package kvbuilder

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

// TODO(VM): Implement at this level some mechanics supporting "effectiveSpec" logic
// TODO(VM): KVVM builder should know which fields are allowed to be changed on-fly, and what params need a new KVVM instance.
// TODO(VM): Somehow report from this layer that "restart is needed" and controller will do other "effectiveSpec"-related stuff.

func NewKVVM(currentKVVM *virtv1.VirtualMachine, opts KVVMOptions) *KVVM {
	return &KVVM{
		kvvm: currentKVVM,
		opts: opts,
	}
}

func NewEmptyKVVM(name types.NamespacedName, opts KVVMOptions) *KVVM {
	res := &KVVM{
		opts: opts,
		kvvm: &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.Name,
				Namespace: name.Namespace,
			},
			Spec: virtv1.VirtualMachineSpec{
				Template: &virtv1.VirtualMachineInstanceTemplateSpec{},
			},
		},
	}
	res.SetCPUModel("Nehalem")
	res.AddNetworkInterface("default")
	return res
}

type KVVMOptions struct {
	EnableParavirtualization bool
	OsType                   virtv2.OsType

	// This option is for local development mode
	ForceBridgeNetworkBinding bool
}

type KVVM struct {
	opts KVVMOptions
	kvvm *virtv1.VirtualMachine
}

func (b *KVVM) SetCPUModel(model string) {
	b.kvvm.Spec.Template.Spec.Domain.CPU = &virtv1.CPU{
		Model: model,
	}
}

func (b *KVVM) SetRunPolicy(runPolicy virtv2.RunPolicy) {
	switch runPolicy {
	case virtv2.AlwaysOnPolicy:
		b.kvvm.Spec.RunStrategy = util.GetPointer(virtv1.RunStrategyAlways)
	case virtv2.AlwaysOffPolicy:
		b.kvvm.Spec.RunStrategy = util.GetPointer(virtv1.RunStrategyHalted)
	case virtv2.ManualPolicy:
		b.kvvm.Spec.RunStrategy = util.GetPointer(virtv1.RunStrategyManual)
	case virtv2.AlwaysOnUnlessStoppedManualy:
		b.kvvm.Spec.RunStrategy = util.GetPointer(virtv1.RunStrategyAlways)
	default:
		panic(fmt.Sprintf("unexpected runPolicy %q", runPolicy))
	}
}

func (b *KVVM) SetResourceRequirements(cores int, coreFraction, memorySize string) {
	_ = coreFraction
	b.kvvm.Spec.Template.Spec.Domain.Resources = virtv1.ResourceRequirements{
		Requests: corev1.ResourceList{
			// FIXME: support coreFraction: req = kvvm.Spec.CPU.Cores * coreFraction
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

func (b *KVVM) AddDisk(name, claimName string) {
	devPreset := DeviceOptionsPresets.Find(b.opts.EnableParavirtualization, b.opts.OsType)
	disk := virtv1.Disk{
		Name: name,
		DiskDevice: virtv1.DiskDevice{
			Disk: &virtv1.DiskTarget{
				Bus: devPreset.DiskBus,
			},
		},
	}
	b.kvvm.Spec.Template.Spec.Domain.Devices.Disks = append(b.kvvm.Spec.Template.Spec.Domain.Devices.Disks, disk)

	volume := virtv1.Volume{
		Name: name,
		VolumeSource: virtv1.VolumeSource{
			PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		},
	}
	b.kvvm.Spec.Template.Spec.Volumes = append(b.kvvm.Spec.Template.Spec.Volumes, volume)
}

func (b *KVVM) AddCdrom(name string) {
	_ = name
	// TODO(VM): implement this helper to attach VMI or CVMI to KV virtual machine
}

func (b *KVVM) AddCloudInit() {
	// TODO(VM): implement this helper to attach cloud-init volume with an initialization data
}

func (b *KVVM) AddNetworkInterface(name string) {
	devPreset := DeviceOptionsPresets.Find(b.opts.EnableParavirtualization, b.opts.OsType)

	foundNetwork := false
	for _, n := range b.kvvm.Spec.Template.Spec.Networks {
		if n.Name == name {
			foundNetwork = true
			break
		}
	}
	if !foundNetwork {
		b.kvvm.Spec.Template.Spec.Networks = append(b.kvvm.Spec.Template.Spec.Networks, virtv1.Network{
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

	b.kvvm.Spec.Template.Spec.Domain.Devices.Interfaces = append(b.kvvm.Spec.Template.Spec.Domain.Devices.Interfaces, i)
}

func (b *KVVM) AddOwnerRef(obj metav1.Object, gvk schema.GroupVersionKind) {
	b.kvvm.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(obj, gvk),
	}
}

func (b *KVVM) AddFinalizer(finalizer string) {
	controllerutil.AddFinalizer(b.kvvm, finalizer)
}

func (b *KVVM) SetBootloader() {
	// TODO(VM): implement bootloader param switch (BIOS used by default in KubeVirt)
}

func (b *KVVM) Resource() *virtv1.VirtualMachine {
	return b.kvvm
}
