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
	"strings"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
)

// Every virtual machine boots the custom image: no cloud-init, SSH as root.
const (
	rootDiskSize = "64Mi"
	dataDiskSize = "50Mi"

	iperfServerVM   = "vm-iperf-server"
	iperfClientVM   = "vm-iperf-client"
	iperfReportPath = "/root/upgrade-iperf-client-report.json"
)

type vmScenario struct {
	name      string
	runPolicy v1alpha2.RunPolicy
}

// dataDiskScenario is a blank disk hotplugged to vm, on STORAGE_CLASS_NAME or,
// when local is set, on LOCAL_STORAGE_CLASS_NAME.
type dataDiskScenario struct {
	name  string
	vm    string
	local bool
}

var vmScenarios = []vmScenario{
	{name: "vm-manual", runPolicy: v1alpha2.ManualPolicy},
	{name: "vm-single-hotplug", runPolicy: v1alpha2.AlwaysOnUnlessStoppedManually},
	{name: "vm-double-hotplug", runPolicy: v1alpha2.AlwaysOnPolicy},
	{name: "vm-replicated-five", runPolicy: v1alpha2.AlwaysOnUnlessStoppedManually},
	{name: "vm-mixed-five", runPolicy: v1alpha2.AlwaysOnUnlessStoppedManually},
	{name: iperfClientVM, runPolicy: v1alpha2.AlwaysOnUnlessStoppedManually},
	{name: iperfServerVM, runPolicy: v1alpha2.AlwaysOnUnlessStoppedManually},
}

var dataDiskScenarios = []dataDiskScenario{
	{name: "vd-data-single-hotplug-01-repl", vm: "vm-single-hotplug"},
	{name: "vd-data-double-hotplug-01-repl", vm: "vm-double-hotplug"},
	{name: "vd-data-double-hotplug-02-local", vm: "vm-double-hotplug", local: true},
	{name: "vd-data-replicated-five-01-repl", vm: "vm-replicated-five"},
	{name: "vd-data-replicated-five-02-repl", vm: "vm-replicated-five"},
	{name: "vd-data-replicated-five-03-repl", vm: "vm-replicated-five"},
	{name: "vd-data-replicated-five-04-repl", vm: "vm-replicated-five"},
	{name: "vd-data-replicated-five-05-repl", vm: "vm-replicated-five"},
	{name: "vd-data-mixed-five-01-repl", vm: "vm-mixed-five"},
	{name: "vd-data-mixed-five-02-repl", vm: "vm-mixed-five"},
	{name: "vd-data-mixed-five-03-local", vm: "vm-mixed-five", local: true},
	{name: "vd-data-mixed-five-04-local", vm: "vm-mixed-five", local: true},
	{name: "vd-data-mixed-five-05-local", vm: "vm-mixed-five", local: true},
	{name: "vd-data-iperf-client-01-repl", vm: iperfClientVM},
	{name: "vd-data-iperf-client-02-repl", vm: iperfClientVM},
}

func findVMScenario(name string) vmScenario {
	for _, s := range vmScenarios {
		if s.name == name {
			return s
		}
	}

	Expect(name).To(BeElementOf(listVMScenarioNames()), "unknown virtual machine scenario")

	return vmScenario{}
}

func listVMScenarioNames() []string {
	names := make([]string, 0, len(vmScenarios))
	for _, s := range vmScenarios {
		names = append(names, s.name)
	}

	return names
}

func findDataDisks(vmName string) []dataDiskScenario {
	var disks []dataDiskScenario

	for _, d := range dataDiskScenarios {
		if d.vm == vmName {
			disks = append(disks, d)
		}
	}

	return disks
}

func (s vmScenario) getRootDiskName() string {
	return "vd-root-" + strings.TrimPrefix(s.name, "vm-")
}

func (s vmScenario) newVMRef() *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: s.name, Namespace: projectName}}
}

func (s vmScenario) newRootDiskRef() *v1alpha2.VirtualDisk {
	return &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: s.getRootDiskName(), Namespace: projectName}}
}

func (s vmScenario) newRootDisk(storageClass string) *v1alpha2.VirtualDisk {
	return object.NewVDFromCVI(s.getRootDiskName(), projectName, object.PrecreatedCVICustomBIOS,
		vdbuilder.WithStorageClass(ptr.To(storageClass)),
		vdbuilder.WithSize(ptr.To(resource.MustParse(rootDiskSize))),
	)
}

func (s vmScenario) newVM() *v1alpha2.VirtualMachine {
	return vmbuilder.New(
		vmbuilder.WithName(s.name),
		vmbuilder.WithNamespace(projectName),
		vmbuilder.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
		vmbuilder.WithMemory(resource.MustParse(object.CustomImageVMMemory)),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithRunPolicy(s.runPolicy),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: s.getRootDiskName(),
		}),
	)
}

func (d dataDiskScenario) getAttachmentName() string {
	return "vmbda-" + strings.TrimPrefix(d.name, "vd-data-")
}

func (d dataDiskScenario) newDiskRef() *v1alpha2.VirtualDisk {
	return &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: d.name, Namespace: projectName}}
}

func (d dataDiskScenario) newAttachmentRef() *v1alpha2.VirtualMachineBlockDeviceAttachment {
	return &v1alpha2.VirtualMachineBlockDeviceAttachment{ObjectMeta: metav1.ObjectMeta{Name: d.getAttachmentName(), Namespace: projectName}}
}

func (d dataDiskScenario) newDisk(storageClass string) *v1alpha2.VirtualDisk {
	return object.NewBlankVD(d.name, projectName, ptr.To(storageClass), ptr.To(resource.MustParse(dataDiskSize)))
}

func (d dataDiskScenario) newAttachment(disk *v1alpha2.VirtualDisk) *v1alpha2.VirtualMachineBlockDeviceAttachment {
	return object.NewVMBDAFromDisk(d.getAttachmentName(), d.vm, disk)
}

func listDiskRefs(s vmScenario) []crclient.Object {
	objects := []crclient.Object{s.newRootDiskRef()}
	for _, d := range findDataDisks(s.name) {
		objects = append(objects, d.newDiskRef())
	}

	return objects
}

func listAttachmentRefs(disks []dataDiskScenario) []crclient.Object {
	objects := make([]crclient.Object, 0, len(disks))
	for _, d := range disks {
		objects = append(objects, d.newAttachmentRef())
	}

	return objects
}
