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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	initialCPUCores     = 1
	initialMemorySize   = "256Mi"
	initialCoreFraction = "5%"
	changedCPUCores     = 2
	changedMemorySize   = "512Mi"
	changedCoreFraction = "10%"
)

var _ = Describe("VirtualMachineConfiguration", func() {
	DescribeTable("the configuration should be applied", func(restartApprovalMode v1alpha2.RestartApprovalMode) {
		f := framework.NewFramework(fmt.Sprintf("vm-configuration-%s", strings.ToLower(string(restartApprovalMode))))
		t := NewConfigurationTest(f)

		DeferCleanup(f.After)
		f.Before()

		By("Environment preparation")
		t.GenerateConfigurationResources(restartApprovalMode)
		err := f.CreateWithDeferredDeletion(context.Background(), t.VM, t.VDRoot, t.VDBlank)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VM agent to be ready")
		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(t.VM), framework.LongTimeout)

		By("Checking initial configuration")
		err = f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
		Expect(err).NotTo(HaveOccurred())
		Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(initialCPUCores))
		Expect(t.VM.Status.Resources.Memory.Size).To(Equal(resource.MustParse(initialMemorySize)))
		Expect(t.VM.Status.Resources.CPU.CoreFraction).To(Equal(initialCoreFraction))
		Expect(t.IsVDAttached(t.VDBlank, t.VM)).To(BeFalse())

		By("Applying changes")
		err = f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
		Expect(err).NotTo(HaveOccurred())
		runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, t.VM.Status.Conditions)
		previousRunningTime := runningCondition.LastTransitionTime.Time

		t.VM.Spec.CPU.Cores = changedCPUCores
		t.VM.Spec.Memory.Size = resource.MustParse(changedMemorySize)
		t.VM.Spec.CPU.CoreFraction = changedCoreFraction
		t.VM.Spec.BlockDeviceRefs = append(t.VM.Spec.BlockDeviceRefs, v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: t.VDBlank.Name,
		})
		err = f.Clients.GenericClient().Update(context.Background(), t.VM)
		Expect(err).NotTo(HaveOccurred())

		t.CheckRestartAwaitingChanges(t.VM)

		if t.VM.Spec.Disruptions.RestartApprovalMode == v1alpha2.Manual {
			util.RebootVirtualMachineBySSH(f, t.VM)
		}

		By("Waiting for VM to be rebooted")
		util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(t.VM), previousRunningTime, framework.LongTimeout)
		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(t.VM), framework.ShortTimeout)

		By("Checking changed configuration")
		err = f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
		Expect(err).NotTo(HaveOccurred())
		Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(changedCPUCores))
		Expect(t.VM.Status.Resources.Memory.Size).To(Equal(resource.MustParse(changedMemorySize)))
		Expect(t.VM.Status.Resources.CPU.CoreFraction).To(Equal(changedCoreFraction))
		Expect(t.IsVDAttached(t.VDBlank, t.VM)).To(BeTrue())
	},
		Entry("when changes are applied manually", v1alpha2.Manual),
		Entry("when changes are applied automatically", v1alpha2.Automatic),
	)
})

type configurationTest struct {
	Framework *framework.Framework

	VM      *v1alpha2.VirtualMachine
	VDRoot  *v1alpha2.VirtualDisk
	VDBlank *v1alpha2.VirtualDisk
}

func NewConfigurationTest(f *framework.Framework) *configurationTest {
	return &configurationTest{
		Framework: f,
	}
}

func (c *configurationTest) GenerateConfigurationResources(restartApprovalMode v1alpha2.RestartApprovalMode) {
	c.VDRoot = vdbuilder.New(
		vdbuilder.WithName("vd-root"),
		vdbuilder.WithNamespace(c.Framework.Namespace().Name),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: object.ImageURLUbuntu,
		}),
	)

	c.VDBlank = vdbuilder.New(
		vdbuilder.WithName("vd-blank"),
		vdbuilder.WithNamespace(c.Framework.Namespace().Name),
		vdbuilder.WithSize(ptr.To(resource.MustParse("100Mi"))),
	)

	c.VM = vmbuilder.New(
		vmbuilder.WithName("vm"),
		vmbuilder.WithNamespace(c.Framework.Namespace().Name),
		vmbuilder.WithCPU(1, ptr.To(initialCoreFraction)),
		vmbuilder.WithMemory(resource.MustParse(initialMemorySize)),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithProvisioningUserData(object.DefaultCloudInit),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: c.VDRoot.Name,
			},
		),
		vmbuilder.WithRestartApprovalMode(restartApprovalMode),
	)
}

func (c *configurationTest) IsVDAttached(vd *v1alpha2.VirtualDisk, vm *v1alpha2.VirtualMachine) bool {
	for _, bd := range vm.Status.BlockDeviceRefs {
		if bd.Kind == v1alpha2.DiskDevice && bd.Name == vd.Name && bd.Attached {
			return true
		}
	}
	return false
}

func (t *configurationTest) CheckRestartAwaitingChanges(vm *v1alpha2.VirtualMachine) {
	if vm.Spec.Disruptions.RestartApprovalMode != v1alpha2.Manual {
		return
	}

	// Avoid race condition with need restart condition calculation
	Eventually(func(g Gomega) {
		err := t.Framework.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
		g.Expect(err).NotTo(HaveOccurred())
		needRestart, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, t.VM.Status.Conditions)
		g.Expect(needRestart.Status).To(Equal(metav1.ConditionTrue))
		g.Expect(t.VM.Status.RestartAwaitingChanges).NotTo(BeNil())
	}).WithTimeout(3 * time.Second).WithPolling(time.Second).Should(Succeed())
}
