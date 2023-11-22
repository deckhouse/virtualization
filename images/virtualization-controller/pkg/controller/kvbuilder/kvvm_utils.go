package kvbuilder

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

const (
	VMDDiskPrefix  = "vmd-"
	VMIDiskPrefix  = "vmi-"
	CVMIDiskPrefix = "cvmi-"
)

func GenerateVMDDiskName(name string) string {
	return VMDDiskPrefix + name
}

func GenerateVMIDiskName(name string) string {
	return VMIDiskPrefix + name
}

func GenerateCVMIDiskName(name string) string {
	return CVMIDiskPrefix + name
}

type HotPlugDeviceSettings struct {
	VolumeName     string
	PVCName        string
	DataVolumeName string
}

func ApplyVirtualMachineSpec(
	kvvm *KVVM, vm *virtv2.VirtualMachine,
	vmdByName map[string]*virtv2.VirtualMachineDisk,
	vmiByName map[string]*virtv2.VirtualMachineImage,
	cvmiByName map[string]*virtv2.ClusterVirtualMachineImage,
	dvcrSettings *dvcr.Settings,
) {
	kvvm.SetRunPolicy(vm.Spec.RunPolicy)
	kvvm.SetOsType(vm.Spec.OsType)
	kvvm.SetBootloader(vm.Spec.Bootloader)
	kvvm.SetCPUModel("Nehalem")
	kvvm.SetNetworkInterface("default")
	kvvm.SetTablet("default-0")
	kvvm.SetNodeSelector(vm.Spec.NodeSelector)
	kvvm.SetTolerations(vm.Spec.Tolerations)
	kvvm.SetAffinity(virtv2.NewAffinityFromVMAffinity(vm.Spec.Affinity))
	kvvm.SetPriorityClassName(vm.Spec.PriorityClassName)
	kvvm.SetTerminationGracePeriod(vm.Spec.TerminationGracePeriodSeconds)
	kvvm.SetTopologySpreadConstraint(vm.Spec.TopologySpreadConstraints)
	// FIXME(VM): real coreFraction
	kvvm.SetResourceRequirements(vm.Spec.CPU.Cores, "", vm.Spec.Memory.Size)

	hotpluggedDevices := make([]HotPlugDeviceSettings, 0)
	for _, volume := range kvvm.Resource.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.Hotpluggable {
			hotpluggedDevices = append(hotpluggedDevices, HotPlugDeviceSettings{
				VolumeName: volume.Name,
				PVCName:    volume.PersistentVolumeClaim.ClaimName,
			})
		}
		// FIXME(VM): not used, now only supports PVC
		if volume.DataVolume != nil && volume.DataVolume.Hotpluggable {
			hotpluggedDevices = append(hotpluggedDevices, HotPlugDeviceSettings{
				VolumeName:     volume.Name,
				DataVolumeName: volume.DataVolume.Name,
			})
		}
	}

	kvvm.ClearDisks()
	for _, bd := range vm.Spec.BlockDevices {
		switch bd.Type {
		case virtv2.ImageDevice:
			// Depending on type Kubernetes|ContainerRegistry we should enable disk or containerDisk

			vmi, hasVMI := vmiByName[bd.VirtualMachineImage.Name]
			if !hasVMI {
				panic(fmt.Sprintf("not found loaded VMI %q which is used in the VM configuration, please report a bug", bd.VirtualMachineImage.Name))
			}
			if vmi.Status.Phase != virtv2.ImageReady {
				panic(fmt.Sprintf("unexpected VMI %q status phase %q: expected ready phase, please report a bug", vmi.Name, vmi.Status.Phase))
			}
			name := GenerateVMIDiskName(bd.VirtualMachineImage.Name)
			switch vmi.Spec.Storage {
			case virtv2.StorageKubernetes:
				kvvm.SetDisk(name, SetDiskOptions{
					PersistentVolumeClaim: util.GetPointer(vmi.Status.Target.PersistentVolumeClaimName),
				})
			case virtv2.StorageContainerRegistry:
				dvcrImage := dvcr.RegistryImageName(dvcrSettings, dvcr.ImagePathForVMI(vmi))
				kvvm.SetDisk(name, SetDiskOptions{
					ContainerDisk: util.GetPointer(dvcrImage),
					IsCdrom:       vmi.Status.CDROM,
				})
			default:
				panic(fmt.Sprintf("unexpected VMI %s spec.storage: %s", vmi.Name, vmi.Spec.Storage))
			}

		case virtv2.ClusterImageDevice:
			// ClusterVirtualMachineImage always has logical type as type=ContainerRegistry (unlinke the VirtualMachineImage)

			cvmi, hasCvmi := cvmiByName[bd.ClusterVirtualMachineImage.Name]
			if !hasCvmi {
				panic(fmt.Sprintf("not found loaded CVMI %q which is used in the VM configuration, please report a bug", bd.ClusterVirtualMachineImage.Name))
			}
			if cvmi.Status.Phase != virtv2.ImageReady {
				panic(fmt.Sprintf("unexpected CVMI %q status phase %q: expected ready phase, please report a bug", cvmi.Name, cvmi.Status.Phase))
			}
			name := GenerateCVMIDiskName(bd.ClusterVirtualMachineImage.Name)
			dvcrImage := dvcr.RegistryImageName(dvcrSettings, dvcr.ImagePathForCVMI(cvmi))
			kvvm.SetDisk(name, SetDiskOptions{
				ContainerDisk: util.GetPointer(dvcrImage),
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
			name := GenerateVMDDiskName(bd.VirtualMachineDisk.Name)
			kvvm.SetDisk(name, SetDiskOptions{
				PersistentVolumeClaim: util.GetPointer(vmd.Status.Target.PersistentVolumeClaimName),
			})

		default:
			panic(fmt.Sprintf("unknown block device type %q", bd.Type))
		}
	}

	for _, device := range hotpluggedDevices {
		switch {
		case device.PVCName != "":
			kvvm.SetDisk(device.VolumeName, SetDiskOptions{
				PersistentVolumeClaim: util.GetPointer(device.PVCName),
				IsHotplugged:          true,
			})
			// FIXME(VM): not used, now only supports PVC
		case device.DataVolumeName != "":
		}
	}
	kvvm.SetCloudInit(vm.Spec.Provisioning)

	kvvm.SetOwnerRef(vm, schema.GroupVersionKind{
		Group:   virtv2.SchemeGroupVersion.Group,
		Version: virtv2.SchemeGroupVersion.Version,
		Kind:    "VirtualMachine",
	})
	kvvm.AddFinalizer(virtv2.FinalizerKVVMProtection)
}
