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

package internal

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	vmservice "github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// hotplugFixture is one block device attached through a VMBDA: the object itself, the volume
// the running instance carries for it and the ref that names it in the VirtualMachine status.
type hotplugFixture struct {
	object    client.Object
	ref       v1alpha2.VMBDAObjectRef
	volume    virtv1.Volume
	statusRef v1alpha2.BlockDeviceStatusRef
}

// A hotplug volume attached through a VMBDA can fall out of the KVVM while the instance keeps
// running it. The KVVM and the KVVMI then stay apart, VolumesSynced never converges and the
// virtual machine cannot migrate until it is restarted, so the sync has to put the volume back
// on its own: attaching a block device does not change the VirtualMachine spec, and nothing
// else will trigger the rewrite.
var _ = Describe("SyncKvvmHandler: hotplug volume dropped from the KVVM", func() {
	const (
		name       = "vm-hotplug-restore"
		namespace  = "default"
		hotplugPVC = "pvc-hotplug"
	)

	var (
		ctx          context.Context
		fakeClient   client.WithWatch
		reconcileObj *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]
		vmState      state.VirtualMachineState
		recorder     *eventrecord.EventRecorderLoggerMock
	)

	BeforeEach(func() {
		ctx = testutil.ContextBackgroundWithNoOpLogger()
		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc:       func(_ client.Object, _, _, _ string) {},
			EventfFunc:      func(_ client.Object, _, _, _ string, _ ...interface{}) {},
			WithLoggingFunc: func(logger eventrecord.InfoLogger) eventrecord.EventRecorderLogger { return recorder },
		}
	})

	attachedDisk := func() hotplugFixture {
		const diskName = "hotplug-disk"
		return hotplugFixture{
			object: &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{Name: diskName, Namespace: namespace, UID: "vd-uid"},
				Status: v1alpha2.VirtualDiskStatus{
					Phase:  v1alpha2.DiskReady,
					Target: v1alpha2.DiskTarget{PersistentVolumeClaim: hotplugPVC},
				},
			},
			ref: v1alpha2.VMBDAObjectRef{Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk, Name: diskName},
			volume: virtv1.Volume{
				Name: kvbuilder.GenerateVDDiskName(diskName),
				VolumeSource: virtv1.VolumeSource{
					PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: hotplugPVC,
						},
						Hotpluggable: true,
					},
				},
			},
			statusRef: v1alpha2.BlockDeviceStatusRef{
				Kind:       v1alpha2.DiskDevice,
				Name:       diskName,
				Hotplugged: true,
			},
		}
	}

	attachedClusterImage := func() hotplugFixture {
		const imageName = "hotplug-image"
		const registryURL = "dvcr.example/image:tag"
		return hotplugFixture{
			object: &v1alpha2.ClusterVirtualImage{
				ObjectMeta: metav1.ObjectMeta{Name: imageName, UID: "cvi-uid"},
				Status: v1alpha2.ClusterVirtualImageStatus{
					Phase:  v1alpha2.ImageReady,
					Target: v1alpha2.ClusterVirtualImageStatusTarget{RegistryURL: registryURL},
				},
			},
			ref: v1alpha2.VMBDAObjectRef{Kind: v1alpha2.VMBDAObjectRefKindClusterVirtualImage, Name: imageName},
			volume: virtv1.Volume{
				Name: kvbuilder.GenerateCVIDiskName(imageName),
				VolumeSource: virtv1.VolumeSource{
					ContainerDisk: &virtv1.ContainerDiskSource{
						Image:        registryURL,
						Hotpluggable: true,
					},
				},
			},
			statusRef: v1alpha2.BlockDeviceStatusRef{
				Kind:       v1alpha2.ClusterImageDevice,
				Name:       imageName,
				Hotplugged: true,
			},
		}
	}

	makeVM := func() *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		vm.Spec.VirtualMachineClassName = "vmclass"
		vm.Spec.CPU.Cores = 2
		vm.Spec.Memory.Size = resource.MustParse("2Gi")
		vm.Spec.RunPolicy = v1alpha2.AlwaysOnPolicy
		vm.Spec.OsType = v1alpha2.GenericOs
		vm.Spec.Disruptions = &v1alpha2.Disruptions{RestartApprovalMode: v1alpha2.Manual}
		vm.Status.Phase = v1alpha2.MachineRunning
		return vm
	}

	// The KVVM carries the spec the controller last applied, so the VirtualMachine has no
	// pending changes: this is the steady state a running VM sits in between user edits.
	makeKVVM := func(vm *v1alpha2.VirtualMachine) *virtv1.VirtualMachine {
		kvvm := &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: virtv1.VirtualMachineSpec{
				RunStrategy: ptr.To(virtv1.RunStrategyAlways),
				Template:    &virtv1.VirtualMachineInstanceTemplateSpec{},
			},
		}
		kvvm.Spec.Template.Spec.Domain.Devices.Interfaces = []virtv1.Interface{
			{Name: network.NameDefaultInterface},
		}
		kvvm.Status.PrintableStatus = virtv1.VirtualMachineStatusRunning
		kvvm.Status.Created = true
		kvvm.Status.Ready = true

		Expect(kvbuilder.SetLastAppliedSpec(kvvm, &v1alpha2.VirtualMachine{Spec: vm.Spec})).To(Succeed())
		Expect(kvbuilder.SetLastAppliedClassSpec(kvvm, &v1alpha2.VirtualMachineClass{
			Spec: v1alpha2.VirtualMachineClassSpec{CPU: v1alpha2.CPU{Type: v1alpha2.CPUTypeHost}},
		})).To(Succeed())

		return kvvm
	}

	makeKVVMI := func(volume virtv1.Volume) *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(name, namespace)
		kvvmi.Status.Phase = virtv1.Running
		kvvmi.Spec.Volumes = []virtv1.Volume{volume}
		kvvmi.Spec.Domain.Devices.Disks = []virtv1.Disk{{
			Name:        volume.Name,
			DiskDevice:  virtv1.DiskDevice{Disk: &virtv1.DiskTarget{Bus: virtv1.DiskBusSCSI}},
			ErrorPolicy: ptr.To(virtv1.DiskErrorPolicyReport),
		}}
		return kvvmi
	}

	makeVMBDA := func(ref v1alpha2.VMBDAObjectRef) *v1alpha2.VirtualMachineBlockDeviceAttachment {
		return &v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{Name: "vmbda-hotplug", Namespace: namespace},
			Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
				VirtualMachineName: name,
				BlockDeviceRef:     ref,
			},
		}
	}

	makeVMClass := func() *v1alpha2.VirtualMachineClass {
		return &v1alpha2.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{Name: "vmclass"},
			Spec:       v1alpha2.VirtualMachineClassSpec{CPU: v1alpha2.CPU{Type: v1alpha2.CPUTypeHost}},
		}
	}

	reconcile := func() {
		h := NewSyncKvvmHandler(nil, fakeClient, recorder, featuregates.Default(),
			vmservice.NewMigrationVolumesService(fakeClient, MakeKVVMFromVMSpec, 10*time.Second))
		_, err := h.Handle(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(reconcileObj.Update(ctx)).To(Succeed())
	}

	kvvmVolumeNames := func() []string {
		kvvm := &virtv1.VirtualMachine{}
		Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, kvvm)).To(Succeed())

		names := make([]string, 0, len(kvvm.Spec.Template.Spec.Volumes))
		for _, volume := range kvvm.Spec.Template.Spec.Volumes {
			names = append(names, volume.Name)
		}
		return names
	}

	// The status is rebuilt from the KVVM volumes, so a dropped volume takes its block device
	// ref down with it: a VM found stuck has neither. Both states have to heal.
	DescribeTable("should restore the volume the instance still runs",
		func(newFixture func() hotplugFixture, keepStatusRef bool) {
			hotplug := newFixture()

			vm := makeVM()
			if keepStatusRef {
				vm.Status.BlockDeviceRefs = []v1alpha2.BlockDeviceStatusRef{hotplug.statusRef}
			}

			fakeClient, reconcileObj, vmState = setupEnvironment(vm,
				makeKVVM(vm),
				makeKVVMI(hotplug.volume),
				hotplug.object,
				makeVMBDA(hotplug.ref),
				makeVMClass(),
			)

			reconcile()

			Expect(kvvmVolumeNames()).To(ContainElement(hotplug.volume.Name))
		},
		Entry("VirtualDisk, still listed in the status", attachedDisk, true),
		Entry("VirtualDisk, gone from the status too", attachedDisk, false),
		Entry("ClusterVirtualImage, still listed in the status", attachedClusterImage, true),
		Entry("ClusterVirtualImage, gone from the status too", attachedClusterImage, false),
	)

	It("should not rewrite the internal virtual machine when nothing drifted", func() {
		// The drift check runs on every reconcile of every running VM, so an unconditional
		// rewrite here would put the whole cluster into an update loop.
		hotplug := attachedDisk()

		vm := makeVM()
		vm.Status.BlockDeviceRefs = []v1alpha2.BlockDeviceStatusRef{hotplug.statusRef}

		kvvm := makeKVVM(vm)
		kvvm.Spec.Template.Spec.Volumes = []virtv1.Volume{hotplug.volume}
		kvvm.Spec.Template.Spec.Domain.Devices.Disks = []virtv1.Disk{{
			Name:        hotplug.volume.Name,
			DiskDevice:  virtv1.DiskDevice{Disk: &virtv1.DiskTarget{Bus: virtv1.DiskBusSCSI}},
			ErrorPolicy: ptr.To(virtv1.DiskErrorPolicyReport),
		}}

		fakeClient, reconcileObj, vmState = setupEnvironment(vm,
			kvvm,
			makeKVVMI(hotplug.volume),
			hotplug.object,
			makeVMBDA(hotplug.ref),
			makeVMClass(),
		)

		before := &virtv1.VirtualMachine{}
		Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, before)).To(Succeed())

		reconcile()

		after := &virtv1.VirtualMachine{}
		Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, after)).To(Succeed())
		Expect(after.ResourceVersion).To(Equal(before.ResourceVersion))
	})
})
