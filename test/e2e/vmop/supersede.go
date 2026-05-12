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

package vmop

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualMachineOperationSupersede", label.Slow(), func() {
	DescribeTable("supersedes active operation", func(runPolicy v1alpha2.RunPolicy, initialPhase v1alpha2.MachinePhase, oldType v1alpha2.VMOPType, oldForce bool, newType v1alpha2.VMOPType, newForce bool, finalPhase v1alpha2.MachinePhase) {
		f := framework.NewFramework(fmt.Sprintf("vmop-supersede-%s-%s", oldType, newType))
		DeferCleanup(f.After)
		f.Before()

		t := newSupersedeTest(f)

		By("Environment preparation", func() {
			t.GenerateResources(runPolicy)
			err := f.CreateWithDeferredDeletion(context.Background(), t.VDRoot, t.VM)
			Expect(err).NotTo(HaveOccurred())
			util.UntilObjectPhase(string(initialPhase), framework.LongTimeout, t.VM)
		})

		By("Creating an active operation", func() {
			t.OldVMOP = t.NewVMOP(oldType, oldForce)
			err := f.CreateWithDeferredDeletion(context.Background(), t.OldVMOP)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Creating a superseding operation", func() {
			t.SupersedingVMOP = t.NewVMOP(newType, newForce)
			err := f.CreateWithDeferredDeletion(context.Background(), t.SupersedingVMOP)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Checking the old operation is marked as superseded", func() {
			t.ExpectSuperseded(t.OldVMOP)
		})

		By("Checking the new operation completes normally", func() {
			util.UntilObjectPhase(string(v1alpha2.VMOPPhaseCompleted), framework.LongTimeout, t.SupersedingVMOP)
			util.UntilObjectPhase(string(finalPhase), framework.MiddleTimeout, t.VM)
		})
	},
		Entry(
			"Start is superseded by Stop",
			v1alpha2.ManualPolicy,
			v1alpha2.MachineStopped,
			v1alpha2.VMOPTypeStart,
			false,
			v1alpha2.VMOPTypeStop,
			false,
			v1alpha2.MachineStopped,
		),
		Entry(
			"Stop is superseded by force Stop",
			v1alpha2.AlwaysOnUnlessStoppedManually,
			v1alpha2.MachineRunning,
			v1alpha2.VMOPTypeStop,
			false,
			v1alpha2.VMOPTypeStop,
			true,
			v1alpha2.MachineStopped,
		),
	)

	It("rejects Start while Stop is active", func() {
		f := framework.NewFramework("vmop-supersede-stop-start")
		DeferCleanup(f.After)
		f.Before()

		t := newSupersedeTest(f)

		By("Environment preparation", func() {
			t.GenerateResources(v1alpha2.AlwaysOnUnlessStoppedManually)
			err := f.CreateWithDeferredDeletion(context.Background(), t.VDRoot, t.VM)
			Expect(err).NotTo(HaveOccurred())
			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, t.VM)
		})

		By("Creating an active Stop operation", func() {
			t.OldVMOP = t.NewVMOP(v1alpha2.VMOPTypeStop, false)
			err := f.CreateWithDeferredDeletion(context.Background(), t.OldVMOP)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Checking Start is rejected by admission", func() {
			newStart := t.NewVMOP(v1alpha2.VMOPTypeStart, false)
			err := f.CreateWithDeferredDeletion(context.Background(), newStart)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("VMOP cannot be executed now"))
			Expect(err.Error()).To(ContainSubstring(t.OldVMOP.Name))
		})
	})
})

type supersedeTest struct {
	Framework *framework.Framework

	VM              *v1alpha2.VirtualMachine
	VDRoot          *v1alpha2.VirtualDisk
	OldVMOP         *v1alpha2.VirtualMachineOperation
	SupersedingVMOP *v1alpha2.VirtualMachineOperation
}

func newSupersedeTest(f *framework.Framework) *supersedeTest {
	return &supersedeTest{
		Framework: f,
	}
}

func (t *supersedeTest) GenerateResources(runPolicy v1alpha2.RunPolicy) {
	t.VDRoot = object.NewVDFromCVI("vd-root", t.Framework.Namespace().Name, object.PrecreatedCVIAlpineBIOS,
		vdbuilder.WithSize(ptr.To(resource.MustParse("512Mi"))),
	)

	t.VM = object.NewMinimalVM("vm-", t.Framework.Namespace().Name,
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: t.VDRoot.Name,
		}),
		vmbuilder.WithRunPolicy(runPolicy),
	)
}

func (t *supersedeTest) NewVMOP(vmopType v1alpha2.VMOPType, force bool) *v1alpha2.VirtualMachineOperation {
	opts := []vmopbuilder.Option{
		vmopbuilder.WithGenerateName(fmt.Sprintf("%s-supersede-%s-", util.VmopE2ePrefix, string(vmopType))),
		vmopbuilder.WithNamespace(t.VM.Namespace),
		vmopbuilder.WithType(vmopType),
		vmopbuilder.WithVirtualMachine(t.VM.Name),
	}

	if force {
		opts = append(opts, vmopbuilder.WithForce(ptr.To(true)))
	}

	return vmopbuilder.New(opts...)
}

func (t *supersedeTest) ExpectSuperseded(vmop *v1alpha2.VirtualMachineOperation) {
	GinkgoHelper()

	util.UntilObjectPhase(string(v1alpha2.VMOPPhaseCompleted), framework.LongTimeout, vmop)
	util.UntilConditionStatus(vmopcondition.TypeCompleted.String(), string(metav1.ConditionTrue), framework.ShortTimeout, vmop)
	util.UntilConditionReason(vmopcondition.TypeCompleted.String(), vmopcondition.ReasonSuperseded.String(), framework.ShortTimeout, vmop)

	err := t.Framework.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vmop), vmop)
	Expect(err).NotTo(HaveOccurred())
	completed, exists := conditions.GetCondition(vmopcondition.TypeCompleted, vmop.Status.Conditions)
	Expect(exists).To(BeTrue())
	Expect(completed.Status).To(Equal(metav1.ConditionTrue))
	Expect(completed.Reason).To(Equal(vmopcondition.ReasonSuperseded.String()))
}
