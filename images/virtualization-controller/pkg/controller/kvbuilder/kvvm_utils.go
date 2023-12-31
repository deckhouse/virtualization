package kvbuilder

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	virtv1 "kubevirt.io/api/core/v1"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/controller/ipam"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/imageformat"
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
	ipAddress string,
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
	kvvm.SetResourceRequirements(vm.Spec.CPU.Cores, vm.Spec.CPU.CoreFraction, vm.Spec.Memory.Size)

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
			// Attach ephemeral disk for storage: Kubernetes.
			// Attach containerDisk for storage: ContainerRegistry (i.e. image from DVCR).

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
				// Attach PVC as ephemeral volume: its data will be restored to initial state on VM restart.
				kvvm.SetDisk(name, SetDiskOptions{
					PersistentVolumeClaim: util.GetPointer(vmi.Status.Target.PersistentVolumeClaimName),
					IsEphemeral:           true,
					Serial:                name,
				})
			case virtv2.StorageContainerRegistry:
				dvcrImage := dvcrSettings.RegistryImageForVMI(vmi.Name, vmi.Namespace)
				kvvm.SetDisk(name, SetDiskOptions{
					ContainerDisk: util.GetPointer(dvcrImage),
					IsCdrom:       imageformat.IsISO(vmi.Status.Format),
					Serial:        name,
				})
			default:
				panic(fmt.Sprintf("unexpected VMI %s spec.storage: %s", vmi.Name, vmi.Spec.Storage))
			}

		case virtv2.ClusterImageDevice:
			// ClusterVirtualMachineImage is attached as containerDisk.

			cvmi, hasCvmi := cvmiByName[bd.ClusterVirtualMachineImage.Name]
			if !hasCvmi {
				panic(fmt.Sprintf("not found loaded CVMI %q which is used in the VM configuration, please report a bug", bd.ClusterVirtualMachineImage.Name))
			}
			if cvmi.Status.Phase != virtv2.ImageReady {
				panic(fmt.Sprintf("unexpected CVMI %q status phase %q: expected ready phase, please report a bug", cvmi.Name, cvmi.Status.Phase))
			}
			name := GenerateCVMIDiskName(bd.ClusterVirtualMachineImage.Name)
			dvcrImage := dvcrSettings.RegistryImageForCVMI(cvmi.Name)
			kvvm.SetDisk(name, SetDiskOptions{
				ContainerDisk: util.GetPointer(dvcrImage),
				IsCdrom:       imageformat.IsISO(cvmi.Status.Format),
				Serial:        name,
			})

		case virtv2.DiskDevice:
			// VirtualMachineDisk is attached as regular disk.

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
				Serial:                name,
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

	// Set ip address cni request annotation.
	kvvm.SetKVVMIAnnotation(ipam.AnnoIPAddressCNIRequest, ipAddress)
	// Set live migration annotation.
	kvvm.SetKVVMIAnnotation(virtv1.AllowPodBridgeNetworkLiveMigrationAnnotation, "true")
}
