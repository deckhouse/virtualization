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
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
)

const (
	replicatedStorageClass = "nested-thin-r1"
	localThinStorageClass  = "nested-local-thin"
	defaultRootDiskSize    = "350Mi"
	defaultDataDiskSize    = "100Mi"
	releaseNamespaceName   = "v12n-test-release"
)

type vmScenario struct {
	name                    string
	rootDiskName            string
	cviName                 string
	cloudInit               string
	runPolicy               v1alpha2.RunPolicy
	rootDiskSize            string
	expectedAdditionalDisks int
	skipGuestAgentCheck     bool

	rootDisk *v1alpha2.VirtualDisk
	vm       *v1alpha2.VirtualMachine
}

type dataDiskScenario struct {
	name         string
	storageClass string
	size         string

	disk *v1alpha2.VirtualDisk
}

type attachmentScenario struct {
	name     string
	vmName   string
	diskName string

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

	test.vms = []*vmScenario{
		{
			name:                    "vm-alpine-manual",
			rootDiskName:            "vd-root-alpine-manual",
			cviName:                 object.PrecreatedCVIAlpineBIOS,
			cloudInit:               object.AlpineCloudInit,
			runPolicy:               v1alpha2.ManualPolicy,
			rootDiskSize:            defaultRootDiskSize,
			expectedAdditionalDisks: 0,
		},
		{
			name:                    "vm-alpine-single-hotplug",
			rootDiskName:            "vd-root-alpine-single-hotplug",
			cviName:                 object.PrecreatedCVIAlpineBIOS,
			cloudInit:               object.AlpineCloudInit,
			runPolicy:               v1alpha2.AlwaysOnUnlessStoppedManually,
			rootDiskSize:            defaultRootDiskSize,
			expectedAdditionalDisks: 1,
		},
		{
			name:                    "vm-alpine-double-hotplug",
			rootDiskName:            "vd-root-alpine-double-hotplug",
			cviName:                 object.PrecreatedCVIAlpineBIOS,
			cloudInit:               object.AlpineCloudInit,
			runPolicy:               v1alpha2.AlwaysOnPolicy,
			rootDiskSize:            defaultRootDiskSize,
			expectedAdditionalDisks: 2,
		},
		{
			name:                    "vm-ubuntu-replicated-five",
			rootDiskName:            "vd-root-ubuntu-replicated-five",
			cviName:                 object.PrecreatedCVIUbuntu,
			cloudInit:               object.UbuntuCloudInit,
			runPolicy:               v1alpha2.AlwaysOnUnlessStoppedManually,
			expectedAdditionalDisks: 5,
		},
		{
			name:                    "vm-ubuntu-mixed-five",
			rootDiskName:            "vd-root-ubuntu-mixed-five",
			cviName:                 object.PrecreatedCVIUbuntu,
			cloudInit:               object.UbuntuCloudInit,
			runPolicy:               v1alpha2.AlwaysOnUnlessStoppedManually,
			expectedAdditionalDisks: 5,
		},
		{
			name:                    "vm-alpine-iperf-client",
			rootDiskName:            "vd-root-alpine-iperf-client",
			cviName:                 object.PrecreatedCVIAlpineBIOS,
			cloudInit:               object.AlpineCloudInit,
			runPolicy:               v1alpha2.AlwaysOnUnlessStoppedManually,
			rootDiskSize:            defaultRootDiskSize,
			expectedAdditionalDisks: 2,
			skipGuestAgentCheck:     true,
		},
		{
			name:                    "vm-alpine-iperf-server",
			rootDiskName:            "vd-root-alpine-iperf-server",
			cviName:                 object.PrecreatedCVIAlpineBIOS,
			cloudInit:               object.PerfCloudInit,
			runPolicy:               v1alpha2.AlwaysOnUnlessStoppedManually,
			rootDiskSize:            defaultRootDiskSize,
			expectedAdditionalDisks: 0,
		},
	}

	test.dataDisks = []*dataDiskScenario{
		{name: "vd-data-alpine-single-hotplug-01-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-alpine-double-hotplug-01-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-alpine-double-hotplug-02-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-replicated-five-01-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-replicated-five-02-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-replicated-five-03-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-replicated-five-04-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-replicated-five-05-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-mixed-five-01-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-mixed-five-02-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-mixed-five-03-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-mixed-five-04-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-ubuntu-mixed-five-05-local", storageClass: localThinStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-alpine-iperf-client-01-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
		{name: "vd-data-alpine-iperf-client-02-repl", storageClass: replicatedStorageClass, size: defaultDataDiskSize},
	}

	test.attachments = []*attachmentScenario{
		{name: "vmbda-alpine-single-hotplug-01", vmName: "vm-alpine-single-hotplug", diskName: "vd-data-alpine-single-hotplug-01-repl"},
		{name: "vmbda-alpine-double-hotplug-01", vmName: "vm-alpine-double-hotplug", diskName: "vd-data-alpine-double-hotplug-01-repl"},
		{name: "vmbda-alpine-double-hotplug-02", vmName: "vm-alpine-double-hotplug", diskName: "vd-data-alpine-double-hotplug-02-local"},
		{name: "vmbda-ubuntu-replicated-five-01", vmName: "vm-ubuntu-replicated-five", diskName: "vd-data-ubuntu-replicated-five-01-repl"},
		{name: "vmbda-ubuntu-replicated-five-02", vmName: "vm-ubuntu-replicated-five", diskName: "vd-data-ubuntu-replicated-five-02-repl"},
		{name: "vmbda-ubuntu-replicated-five-03", vmName: "vm-ubuntu-replicated-five", diskName: "vd-data-ubuntu-replicated-five-03-repl"},
		{name: "vmbda-ubuntu-replicated-five-04", vmName: "vm-ubuntu-replicated-five", diskName: "vd-data-ubuntu-replicated-five-04-repl"},
		{name: "vmbda-ubuntu-replicated-five-05", vmName: "vm-ubuntu-replicated-five", diskName: "vd-data-ubuntu-replicated-five-05-repl"},
		{name: "vmbda-ubuntu-mixed-five-01", vmName: "vm-ubuntu-mixed-five", diskName: "vd-data-ubuntu-mixed-five-01-repl"},
		{name: "vmbda-ubuntu-mixed-five-02", vmName: "vm-ubuntu-mixed-five", diskName: "vd-data-ubuntu-mixed-five-02-repl"},
		{name: "vmbda-ubuntu-mixed-five-03", vmName: "vm-ubuntu-mixed-five", diskName: "vd-data-ubuntu-mixed-five-03-local"},
		{name: "vmbda-ubuntu-mixed-five-04", vmName: "vm-ubuntu-mixed-five", diskName: "vd-data-ubuntu-mixed-five-04-local"},
		{name: "vmbda-ubuntu-mixed-five-05", vmName: "vm-ubuntu-mixed-five", diskName: "vd-data-ubuntu-mixed-five-05-local"},
		{name: "vmbda-alpine-iperf-client-01", vmName: "vm-alpine-iperf-client", diskName: "vd-data-alpine-iperf-client-01-repl"},
		{name: "vmbda-alpine-iperf-client-02", vmName: "vm-alpine-iperf-client", diskName: "vd-data-alpine-iperf-client-02-repl"},
	}

	for _, vmScenario := range test.vms {
		vmScenario.rootDisk = newRootDisk(vmScenario.rootDiskName, namespace, vmScenario.cviName, replicatedStorageClass, vmScenario.rootDiskSize)
		vmScenario.vm = newVM(vmScenario.name, namespace, vmScenario.runPolicy, vmScenario.rootDisk.Name, vmScenario.cloudInit)
		test.vmByName[vmScenario.name] = vmScenario
	}

	for _, diskScenario := range test.dataDisks {
		diskScenario.disk = newHotplugDisk(diskScenario.name, namespace, diskScenario.storageClass, diskScenario.size)
		test.dataDiskByName[diskScenario.name] = diskScenario
	}

	for _, attachmentScenario := range test.attachments {
		vmScenario := test.vmByName[attachmentScenario.vmName]
		diskScenario := test.dataDiskByName[attachmentScenario.diskName]
		attachmentScenario.attachment = object.NewVMBDAFromDisk(attachmentScenario.name, vmScenario.vm.Name, diskScenario.disk)
	}

	test.iperfClient = test.vmByName["vm-alpine-iperf-client"]
	test.iperfServer = test.vmByName["vm-alpine-iperf-server"]

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

func newVM(name, namespace string, runPolicy v1alpha2.RunPolicy, rootDiskName, cloudInit string) *v1alpha2.VirtualMachine {
	return vmbuilder.New(
		vmbuilder.WithName(name),
		vmbuilder.WithNamespace(namespace),
		vmbuilder.WithCPU(1, ptr.To("20%")),
		vmbuilder.WithMemory(resource.MustParse("512Mi")),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithProvisioningUserData(cloudInit),
		vmbuilder.WithRunPolicy(runPolicy),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: rootDiskName,
		}),
	)
}
