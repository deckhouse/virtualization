package kvbuilder

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

func ApplyVirtualMachineSpec(
	kvvm *KVVM, vm *virtv2.VirtualMachine,
	vmdByName map[string]*virtv2.VirtualMachineDisk,
	cvmiByName map[string]*virtv2.ClusterVirtualMachineImage,
) {
	kvvm.SetCPUModel("Nehalem")
	kvvm.AddNetworkInterface("default")
	kvvm.SetRunPolicy(vm.Spec.RunPolicy)
	kvvm.AddTablet("default-0")

	// FIXME(VM): real coreFraction
	kvvm.SetResourceRequirements(vm.Spec.CPU.Cores, "", vm.Spec.Memory.Size)

	for _, bd := range vm.Spec.BlockDevices {
		switch bd.Type {
		case virtv2.ImageDevice:
			panic("not implemented")

			// Depending on type Kubernetes|ContainerRegistry we should enable disk or containerDisk

		case virtv2.ClusterImageDevice:
			// ClusterVirtualMachineImage always has logical type as type=ContainerRegistry (unlinke the VirtualMachineImage)

			cvmi, hasCvmi := cvmiByName[bd.ClusterVirtualMachineImage.Name]
			if !hasCvmi {
				panic(fmt.Sprintf("not found loaded CVMI %q which is used in the VM configuration, please report a bug", bd.ClusterVirtualMachineImage.Name))
			}
			if cvmi.Status.Phase != virtv2.ImageReady {
				panic(fmt.Sprintf("unexpected CVMI %q status phase %q: expected ready phase, please report a bug", cvmi.Name, cvmi.Status.Phase))
			}

			kvvm.AddDisk(bd.ClusterVirtualMachineImage.Name, AddDiskOptions{
				ContainerDisk: util.GetPointer(cvmi.Status.Target.RegistryURL),
				IsCdrom:       cvmi.Status.CDROM,
			})

		case virtv2.DiskDevice:
			vmd, hasVmd := vmdByName[bd.VirtualMachineDisk.Name]
			if !hasVmd {
				panic(fmt.Sprintf("not found loaded VMD %q which is used in the VM configuration, please report a bug", bd.VirtualMachineDisk.Name))
			}
			if vmd.Status.Phase != virtv2.DiskReady {
				panic(fmt.Sprintf("unexpected VMD %q status phase %q: expected ready phase, please report a bug", vmd.Name, vmd.Status.Phase))
			}

			kvvm.AddDisk(bd.VirtualMachineDisk.Name, AddDiskOptions{
				PersistentVolumeClaim: util.GetPointer(vmd.Status.Target.PersistentVolumeClaimName),
			})

		default:
			panic(fmt.Sprintf("unknown block device type %q", bd.Type))
		}
	}

	kvvm.AddOwnerRef(vm, schema.GroupVersionKind{
		Group:   virtv2.SchemeGroupVersion.Group,
		Version: virtv2.SchemeGroupVersion.Version,
		Kind:    "VirtualMachine",
	})
	kvvm.AddFinalizer(virtv2.FinalizerKVVMProtection)
}
