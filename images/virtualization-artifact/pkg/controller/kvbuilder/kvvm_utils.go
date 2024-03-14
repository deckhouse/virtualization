package kvbuilder

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	virtv1 "kubevirt.io/api/core/v1"

	virtv2 "github.com/deckhouse/virtualization-controller/api/core/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/ipam"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

const (
	VMDDiskPrefix        = "vmd-"
	VMIDiskPrefix        = "vmi-"
	CVMIDiskPrefix       = "cvmi-"
	NetworkInterfaceName = "default"
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
) error {
	if err := kvvm.SetRunPolicy(vm.Spec.RunPolicy); err != nil {
		return err
	}
	if err := kvvm.SetOsType(vm.Spec.OsType); err != nil {
		return err
	}
	if err := kvvm.SetBootloader(vm.Spec.Bootloader); err != nil {
		return err
	}
	kvvm.SetCPUModel("Nehalem")
	kvvm.SetNetworkInterface(NetworkInterfaceName)
	kvvm.SetTablet("default-0")
	kvvm.SetNodeSelector(vm.Spec.NodeSelector)
	kvvm.SetTolerations(vm.Spec.Tolerations)
	kvvm.SetAffinity(virtv2.NewAffinityFromVMAffinity(vm.Spec.Affinity))
	kvvm.SetPriorityClassName(vm.Spec.PriorityClassName)
	kvvm.SetTerminationGracePeriod(vm.Spec.TerminationGracePeriodSeconds)
	kvvm.SetTopologySpreadConstraint(vm.Spec.TopologySpreadConstraints)
	if err := kvvm.SetResourceRequirements(vm.Spec.CPU.Cores, vm.Spec.CPU.CoreFraction, vm.Spec.Memory.Size); err != nil {
		return err
	}

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

			vmi := vmiByName[bd.VirtualMachineImage.Name]

			name := GenerateVMIDiskName(bd.VirtualMachineImage.Name)
			switch vmi.Spec.Storage {
			case virtv2.StorageKubernetes:
				// Attach PVC as ephemeral volume: its data will be restored to initial state on VM restart.
				if err := kvvm.SetDisk(name, SetDiskOptions{
					PersistentVolumeClaim: util.GetPointer(vmi.Status.Target.PersistentVolumeClaimName),
					IsEphemeral:           true,
					Serial:                name,
				}); err != nil {
					return err
				}
			case virtv2.StorageContainerRegistry:
				dvcrImage := dvcrSettings.RegistryImageForVMI(vmi.Name, vmi.Namespace)
				if err := kvvm.SetDisk(name, SetDiskOptions{
					ContainerDisk: util.GetPointer(dvcrImage),
					IsCdrom:       imageformat.IsISO(vmi.Status.Format),
					Serial:        name,
				}); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unexpected storage type %q for vmi %s. %w", vmi.Spec.Storage, vmi.Name, common.ErrUnknownType)
			}

		case virtv2.ClusterImageDevice:
			// ClusterVirtualMachineImage is attached as containerDisk.

			cvmi := cvmiByName[bd.ClusterVirtualMachineImage.Name]

			name := GenerateCVMIDiskName(bd.ClusterVirtualMachineImage.Name)
			dvcrImage := dvcrSettings.RegistryImageForCVMI(cvmi.Name)
			if err := kvvm.SetDisk(name, SetDiskOptions{
				ContainerDisk: util.GetPointer(dvcrImage),
				IsCdrom:       imageformat.IsISO(cvmi.Status.Format),
				Serial:        name,
			}); err != nil {
				return err
			}

		case virtv2.DiskDevice:
			// VirtualMachineDisk is attached as regular disk.

			vmd := vmdByName[bd.VirtualMachineDisk.Name]

			name := GenerateVMDDiskName(bd.VirtualMachineDisk.Name)
			if err := kvvm.SetDisk(name, SetDiskOptions{
				PersistentVolumeClaim: util.GetPointer(vmd.Status.Target.PersistentVolumeClaimName),
				Serial:                name,
			}); err != nil {
				return err
			}

		default:
			return fmt.Errorf("unknown block device type %q. %w", bd.Type, common.ErrUnknownType)
		}
	}

	for _, device := range hotpluggedDevices {
		switch {
		case device.PVCName != "":
			if err := kvvm.SetDisk(device.VolumeName, SetDiskOptions{
				PersistentVolumeClaim: util.GetPointer(device.PVCName),
				IsHotplugged:          true,
			}); err != nil {
				return err
			}
			// FIXME(VM): not used, now only supports PVC
		case device.DataVolumeName != "":
		}
	}
	if err := kvvm.SetCloudInit(vm.Spec.Provisioning); err != nil {
		return err
	}

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

	return nil
}
