/*
Copyright 2025 Flant JSC

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

package vm

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	cvibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmbdabuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/network"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("PowerState", func() {
	DescribeTable("manages power state of a virtual machine", func(runPolicy v1alpha2.RunPolicy) {
		var namespaceSuffix string
		switch runPolicy {
		case v1alpha2.AlwaysOnPolicy:
			namespaceSuffix = "always-on"
		case v1alpha2.AlwaysOnUnlessStoppedManually:
			namespaceSuffix = "stopped-manually"
		case v1alpha2.ManualPolicy:
			namespaceSuffix = "manual"
		}
		f := framework.NewFramework(fmt.Sprintf("power-state-%s", namespaceSuffix))
		DeferCleanup(f.After)
		f.Before()

		t := newPowerStateTest(f)

		By("Environment preparation", func() {
			t.GenerateResources(runPolicy)
			err := f.CreateWithDeferredDeletion(
				context.Background(), t.CVI, t.VI, t.VDRoot, t.VDBlank, t.VM, t.VMBDA,
			)
			Expect(err).NotTo(HaveOccurred())

			if t.VM.Spec.RunPolicy == v1alpha2.ManualPolicy {
				util.UntilObjectPhase(string(v1alpha2.MachineStopped), framework.LongTimeout, t.VM)
				util.StartVirtualMachine(f, t.VM)
			}

			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, t.VM)
			util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.MiddleTimeout, t.VMBDA)
			util.UntilSSHReady(f, t.VM, framework.ShortTimeout)
		})

		By("Shutdown VM by VMOP", func() {
			err := f.CreateWithDeferredDeletion(context.Background(), t.VMOPStop)
			Expect(err).NotTo(HaveOccurred())

			switch t.VM.Spec.RunPolicy {
			case v1alpha2.AlwaysOnPolicy:
				util.UntilObjectPhase(string(v1alpha2.VMOPPhaseFailed), framework.ShortTimeout, t.VMOPStop)
				util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.ShortTimeout, t.VM)
				util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout, t.VMBDA)
			case v1alpha2.AlwaysOnUnlessStoppedManually, v1alpha2.ManualPolicy:
				util.UntilObjectPhase(string(v1alpha2.VMOPPhaseCompleted), framework.LongTimeout, t.VMOPStop)
				util.UntilObjectPhase(string(v1alpha2.MachineStopped), framework.ShortTimeout, t.VM)
			}
		})

		By("Start VM", func() {
			if t.VM.Spec.RunPolicy != v1alpha2.AlwaysOnPolicy {
				util.StartVirtualMachine(f, t.VM)
				util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.MiddleTimeout, t.VM)
				util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout, t.VMBDA)
				util.UntilSSHReady(f, t.VM, framework.ShortTimeout)
			}
		})

		By("Shutdown VM by SSH", func() {
			err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
			Expect(err).NotTo(HaveOccurred())
			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, t.VM.Status.Conditions)
			runningLastTransitionTime := runningCondition.LastTransitionTime.Time

			util.StopVirtualMachineFromOS(f, t.VM)

			switch t.VM.Spec.RunPolicy {
			case v1alpha2.AlwaysOnPolicy:
				util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(t.VM), runningLastTransitionTime, framework.LongTimeout)
				util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.ShortTimeout, t.VM)
				util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout, t.VMBDA)
				util.UntilSSHReady(f, t.VM, framework.ShortTimeout)
			case v1alpha2.AlwaysOnUnlessStoppedManually, v1alpha2.ManualPolicy:
				util.UntilObjectPhase(string(v1alpha2.MachineStopped), framework.LongTimeout, t.VM)
			}
		})

		By("Start VM", func() {
			if t.VM.Spec.RunPolicy != v1alpha2.AlwaysOnPolicy {
				util.StartVirtualMachine(f, t.VM)
				util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.MiddleTimeout, t.VM)
				util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout, t.VMBDA)
				util.UntilSSHReady(f, t.VM, framework.ShortTimeout)
			}
		})

		By("Reboot VM by VMOP", func() {
			err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
			Expect(err).NotTo(HaveOccurred())

			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, t.VM.Status.Conditions)
			runningLastTransitionTime := runningCondition.LastTransitionTime.Time

			err = f.CreateWithDeferredDeletion(context.Background(), t.VMOPRestart)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.VMOPPhaseCompleted), framework.LongTimeout, t.VMOPRestart)
			util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(t.VM), runningLastTransitionTime, framework.MiddleTimeout)
			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.ShortTimeout, t.VM)
			util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout, t.VMBDA)
			util.UntilSSHReady(f, t.VM, framework.ShortTimeout)
		})

		By("Reboot VM by SSH", func() {
			err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
			Expect(err).NotTo(HaveOccurred())

			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, t.VM.Status.Conditions)
			runningLastTransitionTime := runningCondition.LastTransitionTime.Time

			util.RebootVirtualMachineBySSH(f, t.VM)

			util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(t.VM), runningLastTransitionTime, framework.LongTimeout)
			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.ShortTimeout, t.VM)
			util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout, t.VMBDA)
			util.UntilSSHReady(f, t.VM, framework.ShortTimeout)
		})

		By("Reboot VM by Pod Deletion", func() {
			err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
			Expect(err).NotTo(HaveOccurred())

			runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, t.VM.Status.Conditions)
			runningLastTransitionTime := runningCondition.LastTransitionTime.Time

			util.RebootVirtualMachineByPodDeletion(f, t.VM)

			if t.VM.Spec.RunPolicy != v1alpha2.AlwaysOnPolicy {
				util.UntilObjectPhase(string(v1alpha2.MachineStopped), framework.MiddleTimeout, t.VM)
				util.StartVirtualMachine(f, t.VM)
			}

			util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(t.VM), runningLastTransitionTime, framework.LongTimeout)
			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.ShortTimeout, t.VM)
			util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.ShortTimeout, t.VMBDA)
			util.UntilSSHReady(f, t.VM, framework.ShortTimeout)
		})

		By("Check VM can reach external network", func() {
			err := network.CheckCiliumAgents(context.Background(), f.Clients.Kubectl(), t.VM.Name, f.Namespace().Name)
			Expect(err).NotTo(HaveOccurred(), "Cilium agents check should succeed for VM %s", t.VM.Name)
			network.CheckExternalConnectivity(f, t.VM.Name, network.ExternalHost, network.HTTPStatusOk)
		})
	},
		Entry(
			"AlwaysOn run policy",
			v1alpha2.AlwaysOnPolicy,
		),
		Entry(
			"Manual run policy",
			v1alpha2.ManualPolicy,
		),
		Entry(
			"AlwaysOnUnlessStoppedManually run policy",
			v1alpha2.AlwaysOnUnlessStoppedManually,
		),
	)
})

type powerStateTest struct {
	Framework *framework.Framework

	CVI         *v1alpha2.ClusterVirtualImage
	VI          *v1alpha2.VirtualImage
	VM          *v1alpha2.VirtualMachine
	VDRoot      *v1alpha2.VirtualDisk
	VDBlank     *v1alpha2.VirtualDisk
	VMBDA       *v1alpha2.VirtualMachineBlockDeviceAttachment
	VMOPStop    *v1alpha2.VirtualMachineOperation
	VMOPStart   *v1alpha2.VirtualMachineOperation
	VMOPRestart *v1alpha2.VirtualMachineOperation
}

func newPowerStateTest(f *framework.Framework) *powerStateTest {
	return &powerStateTest{
		Framework: f,
	}
}

func (t *powerStateTest) GenerateResources(runPolicy v1alpha2.RunPolicy) {
	t.CVI = cvibuilder.New(
		cvibuilder.WithName(fmt.Sprintf("%s-cvi", t.Framework.Namespace().Name)),
		cvibuilder.WithDataSourceHTTP(object.ImageURLMinimalISO, nil, nil),
	)

	t.VI = vibuilder.New(
		vibuilder.WithName("vi"),
		vibuilder.WithNamespace(t.Framework.Namespace().Name),
		vibuilder.WithDataSourceHTTP(object.ImageURLMinimalQCOW, nil, nil),
		vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
	)

	t.VDRoot = vdbuilder.New(
		vdbuilder.WithName("vd-root"),
		vdbuilder.WithNamespace(t.Framework.Namespace().Name),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: object.ImageURLUbuntu,
		}),
	)

	t.VDBlank = vdbuilder.New(
		vdbuilder.WithName("vd-blank"),
		vdbuilder.WithNamespace(t.Framework.Namespace().Name),
		vdbuilder.WithPersistentVolumeClaim(nil, ptr.To(resource.MustParse("51Mi"))),
	)

	t.VM = vmbuilder.New(
		vmbuilder.WithName("vm"),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithCPU(1, ptr.To("10%")),
		vmbuilder.WithMemory(resource.MustParse("512Mi")),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: t.VDRoot.Name,
			},
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ClusterImageDevice,
				Name: t.CVI.Name,
			},
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.ImageDevice,
				Name: t.VI.Name,
			},
		),
		vmbuilder.WithRestartApprovalMode(v1alpha2.Manual),
		vmbuilder.WithRunPolicy(runPolicy),
		vmbuilder.WithProvisioningUserData(object.DefaultCloudInit),
	)

	t.VMBDA = vmbdabuilder.New(
		vmbdabuilder.WithName("vmbda"),
		vmbdabuilder.WithNamespace(t.VDBlank.Namespace),
		vmbdabuilder.WithVirtualMachineName(t.VM.Name),
		vmbdabuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, t.VDBlank.Name),
	)

	t.VMOPStop = vmopbuilder.New(
		vmopbuilder.WithGenerateName("vmop-stop-"),
		vmopbuilder.WithNamespace(t.VM.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeStop),
		vmopbuilder.WithVirtualMachine(t.VM.Name),
	)

	t.VMOPStart = vmopbuilder.New(
		vmopbuilder.WithGenerateName("vmop-start-"),
		vmopbuilder.WithNamespace(t.VM.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeStart),
		vmopbuilder.WithVirtualMachine(t.VM.Name),
	)

	t.VMOPRestart = vmopbuilder.New(
		vmopbuilder.WithGenerateName("vmop-restart-"),
		vmopbuilder.WithNamespace(t.VM.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeRestart),
		vmopbuilder.WithVirtualMachine(t.VM.Name),
	)
}
