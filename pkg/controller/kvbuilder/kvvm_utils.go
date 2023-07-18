package kvbuilder

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

func ApplyVirtualMachineSpec(kvvm *KVVM, vm *virtv2.VirtualMachine, vmdByName map[string]*virtv2.VirtualMachineDisk) {
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

		case virtv2.DiskDevice:
			vmd, hasVmd := vmdByName[bd.VirtualMachineDisk.Name]
			if !hasVmd {
				panic(fmt.Sprintf("not found loaded VMD %q which is used in the VM configuration", bd.VirtualMachineDisk.Name))
			}
			if vmd.Status.Phase != virtv2.DiskReady {
				panic(fmt.Sprintf("unexpected VMD %q status phase %q: expected ready phase", vmd.Name, vmd.Status.Phase))
			}

			kvvm.AddDisk(bd.VirtualMachineDisk.Name, vmd.Status.Target.PersistentVolumeClaimName)

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
