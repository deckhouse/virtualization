/*
Copyright 2026 Flant JSC

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

package release

import (
	"slices"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
)

// The storage classes default to the ones the release pipeline creates in the
// nested cluster; run-local.sh overrides them to run the suite in any cluster.
var (
	replicatedStorageClass = envOrDefault("RELEASE_STORAGE_CLASS_REPLICATED", "nested-thin-r1")
	localThinStorageClass  = envOrDefault("RELEASE_STORAGE_CLASS_LOCAL", "nested-local-thin")
	nfsStorageClass        = envOrDefault("RELEASE_STORAGE_CLASS_NFS", "nfs")
)

const (
	// Root disk sizes match the source images: the Alpine cloud image is tiny,
	// RedOS/Ubuntu/Windows server images need real room to unpack and grow
	// into.
	alpineRootDiskSize  = "1Gi"
	redosRootDiskSize   = "8Gi"
	ubuntuRootDiskSize  = "5Gi"
	windowsRootDiskSize = "20Gi"

	// releaseVMMemory is the floor proven elsewhere in this suite for a full
	// Alpine/Ubuntu cloud image plus OVMF: below it the guest agent does not
	// come up reliably. RedOS and Windows get more, matching what the source
	// manual cluster runs them with.
	releaseVMMemory     = "512Mi"
	redosVMMemory       = "1Gi"
	ubuntuServerVMemory = "1Gi"
	windowsVMMemory     = "2Gi"

	windowsSysprepSecretName  = "win2022core-unattend-w-iperf-client"
	windowsIperfJQToolsVIName = "vi-iperf-jq-tools"

	ubuntuTestISOVIName  = "vi-test-iso-u-iperf-server"
	ubuntuTestQCOWVIName = "vi-pvc-test-qcow-u-iperf-server"
	ubuntuTestQCOWVDName = "vd-test-qcow-u-iperf-server"

	defaultDataDiskSize = "75Mi"
	blankDiskSize       = "100Mi"
)

type vmScenario struct {
	name                 string
	rootDiskName         string
	cviName              string
	cloudInit            string
	osType               v1alpha2.OsType
	bootloader           v1alpha2.BootloaderType
	cpuCores             int
	cpuCoreFraction      string
	memory               string
	runPolicy            v1alpha2.RunPolicy
	rootDiskStorageClass string
	rootDiskSize         string
	skipGuestAgentCheck  bool
	// skipDiskCheck opts a scenario out of the guest-side part of
	// expectBlockDevices, whose lsblk/findmnt-based counting only understands
	// a Linux guest. The VM status part still runs.
	skipDiskCheck bool
	// extraBlockDeviceRefs are attached at creation alongside the root disk
	// (spec.blockDeviceRefs), as opposed to the hotplugged attachments.
	extraBlockDeviceRefs []v1alpha2.BlockDeviceSpecRef
	// sysprepSecretName selects SysprepRef provisioning instead of the
	// UserData cloud-init in cloudInit; the two are mutually exclusive.
	sysprepSecretName string
	// customImage marks a scenario booted from the custom image: no
	// cloud-init (cloudInit must be empty), guest commands log in as root
	// (dropbear, baked key) without sudo. None of the current scenarios use
	// it; every VM here boots a real distro image instead.
	customImage bool

	rootDisk *v1alpha2.VirtualDisk
	vm       *v1alpha2.VirtualMachine
	// expectedBlockDevices is every block device the VM must have attached:
	// its spec.blockDeviceRefs plus the hotplugged attachments targeting it.
	expectedBlockDevices []v1alpha2.BlockDeviceSpecRef

	// vmObs is armed before the VM is created (pre-upgrade phase only), so
	// phase and agent transitions are captured from the very beginning.
	vmObs vmobs.Observer
}

type dataDiskScenario struct {
	name         string
	storageClass string
	size         string

	disk *v1alpha2.VirtualDisk
}

type attachmentScenario struct {
	name   string
	vmName string
	// kind defaults to VirtualDisk when empty.
	kind    v1alpha2.VMBDAObjectRefKind
	refName string

	attachment *v1alpha2.VirtualMachineBlockDeviceAttachment
}

func (s *vmScenario) expectedInitialPhase() string {
	if s.runPolicy == v1alpha2.ManualPolicy {
		return string(v1alpha2.MachineStopped)
	}

	return string(v1alpha2.MachineRunning)
}

func newCurrentReleaseSmokeTest(f *framework.Framework, namespace string) *currentReleaseSmokeTest {
	Expect(namespace).NotTo(BeEmpty(), "namespace must be provided")

	test := &currentReleaseSmokeTest{
		framework:      f,
		vmByName:       make(map[string]*vmScenario),
		dataDiskByName: make(map[string]*dataDiskScenario),
	}

	// The scenario set mirrors the VMs of a manually maintained
	// update-virtualization-version smoke deployment (Alpine in several
	// root/hotplug storage-class combinations, plus one RedOS server, one
	// Ubuntu server and one Windows client), so the release smoke exercises
	// the same OS/storage-class diversity instead of a single synthetic
	// image. Disks, images and attachments follow that deployment's
	// kustomize template one to one. The Windows entry skips the guest-side
	// part of expectBlockDevices (skipDiskCheck): it is lsblk/findmnt-based
	// and only understands a Linux guest.
	test.vms = []*vmScenario{
		{
			name:                 "vm-alpine-a-iperf-client",
			rootDiskName:         "vd-root-alpine-a-iperf-client",
			cviName:              object.PrecreatedCVIAlpineUEFI,
			cloudInit:            object.AlpineCloudInit,
			bootloader:           v1alpha2.EFI,
			cpuCores:             1,
			cpuCoreFraction:      "20%",
			memory:               releaseVMMemory,
			runPolicy:            v1alpha2.AlwaysOnUnlessStoppedManually,
			rootDiskStorageClass: replicatedStorageClass,
			rootDiskSize:         alpineRootDiskSize,
			skipGuestAgentCheck:  true,
			extraBlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd-blank-a-iperf-client"},
			},
		},
		{
			name:                 "vm-alpine-local",
			rootDiskName:         "vd-root-alpine-local",
			cviName:              object.PrecreatedCVIAlpineUEFI,
			cloudInit:            object.AlpineCloudInit,
			bootloader:           v1alpha2.EFI,
			cpuCores:             1,
			cpuCoreFraction:      "20%",
			memory:               releaseVMMemory,
			runPolicy:            v1alpha2.AlwaysOnUnlessStoppedManually,
			rootDiskStorageClass: localThinStorageClass,
			rootDiskSize:         alpineRootDiskSize,
		},
		{
			name:                 "vm-alpine-local-hp-local",
			rootDiskName:         "vd-root-alpine-local-hp-local",
			cviName:              object.PrecreatedCVIAlpineUEFI,
			cloudInit:            object.AlpineCloudInit,
			bootloader:           v1alpha2.EFI,
			cpuCores:             1,
			cpuCoreFraction:      "20%",
			memory:               releaseVMMemory,
			runPolicy:            v1alpha2.AlwaysOnUnlessStoppedManually,
			rootDiskStorageClass: localThinStorageClass,
			rootDiskSize:         alpineRootDiskSize,
		},
		{
			name:                 "vm-alpine-local-hp-replica",
			rootDiskName:         "vd-root-alpine-local-hp-replica",
			cviName:              object.PrecreatedCVIAlpineUEFI,
			cloudInit:            object.AlpineCloudInit,
			bootloader:           v1alpha2.EFI,
			cpuCores:             1,
			cpuCoreFraction:      "20%",
			memory:               releaseVMMemory,
			runPolicy:            v1alpha2.AlwaysOnUnlessStoppedManually,
			rootDiskStorageClass: localThinStorageClass,
			rootDiskSize:         alpineRootDiskSize,
		},
		{
			name:                 "vm-alpine-nfs-hp-nfs",
			rootDiskName:         "vd-root-alpine-nfs-hp-nfs",
			cviName:              object.PrecreatedCVIAlpineUEFI,
			cloudInit:            object.AlpineCloudInit,
			bootloader:           v1alpha2.EFI,
			cpuCores:             1,
			cpuCoreFraction:      "20%",
			memory:               releaseVMMemory,
			runPolicy:            v1alpha2.AlwaysOnUnlessStoppedManually,
			rootDiskStorageClass: nfsStorageClass,
			rootDiskSize:         alpineRootDiskSize,
		},
		{
			name:                 "vm-alpine-repl-local-nfs-hotplug-repl-local-nfs",
			rootDiskName:         "vd-root-alpine-repl-local-nfs-hotplug-repl-local-nfs",
			cviName:              object.PrecreatedCVIAlpineUEFI,
			cloudInit:            object.AlpineCloudInit,
			bootloader:           v1alpha2.EFI,
			cpuCores:             1,
			cpuCoreFraction:      "20%",
			memory:               releaseVMMemory,
			runPolicy:            v1alpha2.AlwaysOnUnlessStoppedManually,
			rootDiskStorageClass: localThinStorageClass,
			rootDiskSize:         alpineRootDiskSize,
			extraBlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd-blank-repl-local-nfs-hotplug-repl-local-nfs"},
				{Kind: v1alpha2.DiskDevice, Name: "vd-blank-nfs-repl-local-nfs-hotplug-repl-local-nfs"},
			},
		},
		{
			name:                 "vm-alpine-replica-hp-local",
			rootDiskName:         "vd-root-alpine-replica-hp-local",
			cviName:              object.PrecreatedCVIAlpineUEFI,
			cloudInit:            object.AlpineCloudInit,
			bootloader:           v1alpha2.EFI,
			cpuCores:             1,
			cpuCoreFraction:      "20%",
			memory:               releaseVMMemory,
			runPolicy:            v1alpha2.AlwaysOnUnlessStoppedManually,
			rootDiskStorageClass: replicatedStorageClass,
			rootDiskSize:         alpineRootDiskSize,
		},
		{
			// The one scenario left on ManualPolicy: it exercises the
			// "started by the smoke, not by the RunPolicy" path that
			// manualStartVMs/expectedInitialPhase cover below.
			name:                 "vm-alpine-should-be-stopped",
			rootDiskName:         "vd-root-alpine-should-be-stopped",
			cviName:              object.PrecreatedCVIAlpineUEFI,
			cloudInit:            object.AlpineCloudInit,
			bootloader:           v1alpha2.EFI,
			cpuCores:             1,
			cpuCoreFraction:      "20%",
			memory:               releaseVMMemory,
			runPolicy:            v1alpha2.ManualPolicy,
			rootDiskStorageClass: replicatedStorageClass,
			rootDiskSize:         alpineRootDiskSize,
			extraBlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
				{Kind: v1alpha2.DiskDevice, Name: "vd-blank-should-be-stopped"},
			},
		},
		{
			// iperf3 is installed via cloud-init (see startLongRunningIPerf);
			// nothing starts it at boot.
			name:                 "vm-redos-r-iperf-server",
			rootDiskName:         "vd-root-redos-r-iperf-server",
			cviName:              object.PrecreatedCVIRedOS,
			cloudInit:            object.RedOSCloudInit,
			bootloader:           v1alpha2.BIOS,
			cpuCores:             1,
			cpuCoreFraction:      "20%",
			memory:               redosVMMemory,
			runPolicy:            v1alpha2.AlwaysOnUnlessStoppedManually,
			rootDiskStorageClass: replicatedStorageClass,
			rootDiskSize:         redosRootDiskSize,
		},
		{
			// A bystander VM: present and verified like every other
			// scenario, but not part of the iperf client/server pair. It
			// carries the hotplugged ISO image and the disk restored from a
			// PVC-backed image: see the attachments below.
			name:                 "vm-ub24-u-iperf-server",
			rootDiskName:         "vd-root-ub24-u-iperf-server",
			cviName:              object.PrecreatedCVIUbuntu,
			cloudInit:            object.UbuntuCloudInit,
			bootloader:           v1alpha2.EFI,
			cpuCores:             1,
			cpuCoreFraction:      "50%",
			memory:               ubuntuServerVMemory,
			runPolicy:            v1alpha2.AlwaysOnPolicy,
			rootDiskStorageClass: replicatedStorageClass,
			rootDiskSize:         ubuntuRootDiskSize,
		},
		{
			// Sysprep provisioning (not cloud-init) and two hotplugged ISOs
			// (virtio drivers, iperf3/jq tools): see the extraObjects and
			// attachments built below.
			name:                 "vm-win2022-w-iperf-client",
			rootDiskName:         "vd-c-win2022-w-iperf-client",
			cviName:              object.PrecreatedCVIWin2022Core,
			osType:               v1alpha2.Windows,
			bootloader:           v1alpha2.EFIWithSecureBoot,
			cpuCores:             4,
			cpuCoreFraction:      "50%",
			memory:               windowsVMMemory,
			runPolicy:            v1alpha2.AlwaysOnPolicy,
			rootDiskStorageClass: replicatedStorageClass,
			rootDiskSize:         windowsRootDiskSize,
			sysprepSecretName:    windowsSysprepSecretName,
			// qemu-guest-agent for Windows needs its MSI installed inside the
			// image; unconfirmed for this one, so don't gate on it.
			skipGuestAgentCheck: true,
			skipDiskCheck:       true,
		},
	}

	testQCOWVI := vibuilder.New(
		vibuilder.WithName(ubuntuTestQCOWVIName),
		vibuilder.WithNamespace(namespace),
		vibuilder.WithDataSourceHTTP(object.ImageTestDataQCOW, nil, nil),
		vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
	)
	testQCOWVI.Spec.PersistentVolumeClaim.StorageClass = ptr.To(replicatedStorageClass)

	test.extraObjects = []crclient.Object{
		object.NewWindows2022SysprepSecret(windowsSysprepSecretName, namespace),
		vibuilder.New(
			vibuilder.WithName(windowsIperfJQToolsVIName),
			vibuilder.WithNamespace(namespace),
			vibuilder.WithDataSourceHTTP(object.ImageURLWinIperfJQTools, nil, nil),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
		),
		vibuilder.New(
			vibuilder.WithName(ubuntuTestISOVIName),
			vibuilder.WithNamespace(namespace),
			vibuilder.WithDataSourceHTTP(object.ImageTestDataISO, nil, nil),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
		),
		testQCOWVI,
		object.NewVDFromVI(ubuntuTestQCOWVDName, namespace, testQCOWVI,
			vdbuilder.WithStorageClass(ptr.To(replicatedStorageClass)),
		),
	}

	test.dataDisks = []*dataDiskScenario{
		{name: "vd-blank-a-iperf-client", storageClass: replicatedStorageClass, size: blankDiskSize},
		{name: "vd-blank-repl-local-nfs-hotplug-repl-local-nfs", storageClass: replicatedStorageClass, size: blankDiskSize},
		{name: "vd-blank-nfs-repl-local-nfs-hotplug-repl-local-nfs", storageClass: nfsStorageClass, size: blankDiskSize},
		{name: "vd-blank-should-be-stopped", storageClass: replicatedStorageClass, size: blankDiskSize},
		{name: "vd-test-1-local-hp-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-2-local-hp-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-3-local-hp-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-4-local-hp-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-5-local-hp-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-1-local-hp-replica", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-2-local-hp-replica", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-3-local-hp-replica", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-4-local-hp-replica", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-5-local-hp-replica", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-1-nfs-hp-nfs", storageClass: nfsStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-1-repl-local-nfs-hotplug-repl-local-nfs", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-2-repl-local-nfs-hotplug-repl-local-nfs", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-3-repl-local-nfs-hotplug-repl-local-nfs", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-4-repl-local-nfs-hotplug-repl-local-nfs", storageClass: nfsStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-1-replica-hp-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-2-replica-hp-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-3-replica-hp-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-4-replica-hp-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-test-5-replica-hp-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
	}

	test.attachments = []*attachmentScenario{
		{name: "vmbda-test-1-local-hp-local", vmName: "vm-alpine-local-hp-local", refName: "vd-test-1-local-hp-local"},
		{name: "vmbda-test-2-local-hp-local", vmName: "vm-alpine-local-hp-local", refName: "vd-test-2-local-hp-local"},
		{name: "vmbda-test-3-local-hp-local", vmName: "vm-alpine-local-hp-local", refName: "vd-test-3-local-hp-local"},
		{name: "vmbda-test-4-local-hp-local", vmName: "vm-alpine-local-hp-local", refName: "vd-test-4-local-hp-local"},
		{name: "vmbda-test-5-local-hp-local", vmName: "vm-alpine-local-hp-local", refName: "vd-test-5-local-hp-local"},
		{name: "vmbda-test-1-local-hp-replica", vmName: "vm-alpine-local-hp-replica", refName: "vd-test-1-local-hp-replica"},
		{name: "vmbda-test-2-local-hp-replica", vmName: "vm-alpine-local-hp-replica", refName: "vd-test-2-local-hp-replica"},
		{name: "vmbda-test-3-local-hp-replica", vmName: "vm-alpine-local-hp-replica", refName: "vd-test-3-local-hp-replica"},
		{name: "vmbda-test-4-local-hp-replica", vmName: "vm-alpine-local-hp-replica", refName: "vd-test-4-local-hp-replica"},
		{name: "vmbda-test-5-local-hp-replica", vmName: "vm-alpine-local-hp-replica", refName: "vd-test-5-local-hp-replica"},
		{name: "vmbda-test-1-nfs-hp-nfs", vmName: "vm-alpine-nfs-hp-nfs", refName: "vd-test-1-nfs-hp-nfs"},
		{name: "vmbda-test-1-repl-local-nfs-hotplug-repl-local-nfs", vmName: "vm-alpine-repl-local-nfs-hotplug-repl-local-nfs", refName: "vd-test-1-repl-local-nfs-hotplug-repl-local-nfs"},
		{name: "vmbda-test-2-repl-local-nfs-hotplug-repl-local-nfs", vmName: "vm-alpine-repl-local-nfs-hotplug-repl-local-nfs", refName: "vd-test-2-repl-local-nfs-hotplug-repl-local-nfs"},
		{name: "vmbda-test-3-repl-local-nfs-hotplug-repl-local-nfs", vmName: "vm-alpine-repl-local-nfs-hotplug-repl-local-nfs", refName: "vd-test-3-repl-local-nfs-hotplug-repl-local-nfs"},
		{name: "vmbda-test-4-repl-local-nfs-hotplug-repl-local-nfs", vmName: "vm-alpine-repl-local-nfs-hotplug-repl-local-nfs", refName: "vd-test-4-repl-local-nfs-hotplug-repl-local-nfs"},
		{name: "vmbda-test-1-replica-hp-local", vmName: "vm-alpine-replica-hp-local", refName: "vd-test-1-replica-hp-local"},
		{name: "vmbda-test-2-replica-hp-local", vmName: "vm-alpine-replica-hp-local", refName: "vd-test-2-replica-hp-local"},
		{name: "vmbda-test-3-replica-hp-local", vmName: "vm-alpine-replica-hp-local", refName: "vd-test-3-replica-hp-local"},
		{name: "vmbda-test-4-replica-hp-local", vmName: "vm-alpine-replica-hp-local", refName: "vd-test-4-replica-hp-local"},
		{name: "vmbda-test-5-replica-hp-local", vmName: "vm-alpine-replica-hp-local", refName: "vd-test-5-replica-hp-local"},
		{name: "vmbda-test-iso-u-iperf-server", vmName: "vm-ub24-u-iperf-server", kind: v1alpha2.VMBDAObjectRefKindVirtualImage, refName: ubuntuTestISOVIName},
		{name: "vmbda-test-qcow-u-iperf-server", vmName: "vm-ub24-u-iperf-server", refName: ubuntuTestQCOWVDName},
		{name: "vmbda-virtio-iso-w-iperf-client", vmName: "vm-win2022-w-iperf-client", kind: v1alpha2.VMBDAObjectRefKindClusterVirtualImage, refName: object.PrecreatedCVIVirtioWin},
		{name: "vmbda-vi-iperf-jq-tools-w-iperf-client", vmName: "vm-win2022-w-iperf-client", kind: v1alpha2.VMBDAObjectRefKindVirtualImage, refName: windowsIperfJQToolsVIName},
	}

	for _, vmScenario := range test.vms {
		vmScenario.rootDisk = newRootDisk(vmScenario.rootDiskName, namespace, vmScenario.cviName, vmScenario.rootDiskStorageClass, vmScenario.rootDiskSize)
		vmScenario.vm = newVM(vmScenario, namespace)
		vmScenario.expectedBlockDevices = slices.Clone(vmScenario.vm.Spec.BlockDeviceRefs)
		test.vmByName[vmScenario.name] = vmScenario
	}

	for _, diskScenario := range test.dataDisks {
		diskScenario.disk = newHotplugDisk(diskScenario.name, namespace, diskScenario.storageClass, diskScenario.size)
		test.dataDiskByName[diskScenario.name] = diskScenario
	}

	for _, attachmentScenario := range test.attachments {
		if attachmentScenario.kind == "" {
			attachmentScenario.kind = v1alpha2.VMBDAObjectRefKindVirtualDisk
		}
		attachmentScenario.attachment = vmbdabuilder.New(
			vmbdabuilder.WithName(attachmentScenario.name),
			vmbdabuilder.WithNamespace(namespace),
			vmbdabuilder.WithVirtualMachineName(attachmentScenario.vmName),
			vmbdabuilder.WithBlockDeviceRef(attachmentScenario.kind, attachmentScenario.refName),
		)

		vmScenario := test.vmByName[attachmentScenario.vmName]
		vmScenario.expectedBlockDevices = append(vmScenario.expectedBlockDevices, v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.BlockDeviceKind(attachmentScenario.kind),
			Name: attachmentScenario.refName,
		})
	}

	test.iperfClient = test.vmByName["vm-alpine-a-iperf-client"]
	test.iperfServer = test.vmByName["vm-redos-r-iperf-server"]

	return test
}

func newRootDisk(name, namespace, cviName, storageClass, size string) *v1alpha2.VirtualDisk {
	options := []vdbuilder.Option{
		vdbuilder.WithStorageClass(ptr.To(storageClass)),
	}
	if size != "" {
		options = append(options, vdbuilder.WithSize(ptr.To(resource.MustParse(size))))
	}

	return object.NewVDFromCVI(name, namespace, cviName, options...)
}

func newHotplugDisk(name, namespace, storageClass, size string) *v1alpha2.VirtualDisk {
	return object.NewBlankVD(
		name,
		namespace,
		ptr.To(storageClass),
		ptr.To(resource.MustParse(size)),
	)
}

func newVM(s *vmScenario, namespace string) *v1alpha2.VirtualMachine {
	blockDeviceRefs := append([]v1alpha2.BlockDeviceSpecRef{
		{Kind: v1alpha2.DiskDevice, Name: s.rootDiskName},
	}, s.extraBlockDeviceRefs...)

	opts := []vmbuilder.Option{
		vmbuilder.WithName(s.name),
		vmbuilder.WithNamespace(namespace),
		vmbuilder.WithBootloader(s.bootloader),
		vmbuilder.WithCPU(s.cpuCores, ptr.To(s.cpuCoreFraction)),
		vmbuilder.WithMemory(resource.MustParse(s.memory)),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithRunPolicy(s.runPolicy),
		vmbuilder.WithBlockDeviceRefs(blockDeviceRefs...),
	}
	if s.osType != "" {
		opts = append(opts, vmbuilder.WithOsType(s.osType))
	}

	switch {
	case s.sysprepSecretName != "":
		opts = append(opts, vmbuilder.WithProvisioning(&v1alpha2.Provisioning{
			Type: v1alpha2.ProvisioningTypeSysprepRef,
			SysprepRef: &v1alpha2.SysprepRef{
				Kind: v1alpha2.SysprepRefKindSecret,
				Name: s.sysprepSecretName,
			},
		}))
	case s.cloudInit != "":
		opts = append(opts, vmbuilder.WithProvisioningUserData(s.cloudInit))
	}

	return vmbuilder.New(opts...)
}
