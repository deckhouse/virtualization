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

type VirtualMachine struct {
	opts VirtualMachineOptions
	vm   *virtv1.VirtualMachine
}

type VirtualMachineOptions struct {
	EnableParavirtualization bool
	OsType                   virtv2.OsType

	// This option is for local development mode
	ForceBridgeNetworkBinding bool
}

func NewEmptyVirtualMachine(name types.NamespacedName, opts VirtualMachineOptions) *VirtualMachine {
	labels := map[string]string{}
	annotations := map[string]string{}

	res := &VirtualMachine{
		opts: opts,
		vm: &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name.Name,
				Namespace:   name.Namespace,
				Labels:      labels,
				Annotations: annotations,
			},
			Spec: virtv1.VirtualMachineSpec{
				// TODO(VM): Implement RunPolicy instead
				Running: util.GetPointer(true),
				// RunStrategy: util.GetPointer(virtv1.RunStrategyAlways),
				Template: &virtv1.VirtualMachineInstanceTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{},
					Spec: virtv1.VirtualMachineInstanceSpec{
						Domain: virtv1.DomainSpec{
							CPU: &virtv1.CPU{
								Model: "Nehalem",
							},
						},
					},
				},
			},
		},
	}
	res.AddNetworkInterface("default")

	return res
}

// TODO(VM): implement methods to make changes to existing virtual machine disks
func NewVirtualMachine(currentVM *virtv1.VirtualMachine, opts VirtualMachineOptions) *VirtualMachine {
	return &VirtualMachine{
		vm:   currentVM,
		opts: opts,
	}
}

func (b *VirtualMachine) SetResourceRequirements(cores int, coreFraction, memorySize string) {
	_ = coreFraction
	b.vm.Spec.Template.Spec.Domain.Resources = virtv1.ResourceRequirements{
		Requests: corev1.ResourceList{
			// FIXME: support coreFraction: req = vm.Spec.CPU.Cores * coreFraction
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

func (b *VirtualMachine) AddDisk(name, claimName string) {
	devPreset := DeviceOptionsPresets.Find(b.opts.EnableParavirtualization, b.opts.OsType)
	disk := virtv1.Disk{
		Name: name,
		DiskDevice: virtv1.DiskDevice{
			Disk: &virtv1.DiskTarget{
				Bus: devPreset.DiskBus,
			},
		},
	}
	b.vm.Spec.Template.Spec.Domain.Devices.Disks = append(b.vm.Spec.Template.Spec.Domain.Devices.Disks, disk)

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
	b.vm.Spec.Template.Spec.Volumes = append(b.vm.Spec.Template.Spec.Volumes, volume)
}

func (b *VirtualMachine) AddCdrom(name string) {
	_ = name
	// TODO(VM): implement this helper to attach VMI or CVMI to KV virtual machine
}

func (b *VirtualMachine) AddCloudInit() {
	// TODO(VM): implement this helper to attach cloud-init volume with an initialization data
}

func (b *VirtualMachine) AddNetworkInterface(name string) {
	devPreset := DeviceOptionsPresets.Find(b.opts.EnableParavirtualization, b.opts.OsType)

	foundNetwork := false
	for _, n := range b.vm.Spec.Template.Spec.Networks {
		if n.Name == name {
			foundNetwork = true
			break
		}
	}
	if !foundNetwork {
		b.vm.Spec.Template.Spec.Networks = append(b.vm.Spec.Template.Spec.Networks, virtv1.Network{
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

	b.vm.Spec.Template.Spec.Domain.Devices.Interfaces = append(b.vm.Spec.Template.Spec.Domain.Devices.Interfaces, i)
}

func (b *VirtualMachine) AddOwnerRef(obj metav1.Object, gvk schema.GroupVersionKind) {
	b.vm.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(obj, gvk),
	}
}

func (b *VirtualMachine) AddFinalizer(finalizer string) {
	controllerutil.AddFinalizer(b.vm, finalizer)
}

func (b *VirtualMachine) SetBootloader() {
	// TODO(VM): implement bootloader param switch (BIOS used by default in KubeVirt)
}

func (b *VirtualMachine) Resource() *virtv1.VirtualMachine {
	return b.vm
}
