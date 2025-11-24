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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
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
	initialCPUCores   = 1
	initialMemorySize = "256Mi"
	changedCPUCores   = 2
	changedMemorySize = "512Mi"
)

var _ = Describe("VirtualMachineConfiguration", func() {
	DescribeTable("the configuration should be applied", func(restartApprovalMode v1alpha2.RestartApprovalMode) {
		f := framework.NewFramework(fmt.Sprintf("vm-configuration-%s", strings.ToLower(string(restartApprovalMode))))
		DeferCleanup(f.After)
		f.Before()

		By("Environment preparation")
		vm, vd := generateConfigurationResources(f.Namespace().Name, restartApprovalMode)
		f.CreateWithDeferredDeletion(context.Background(), vm, vd)

		By("Waiting for VM agent to be ready")
		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)

		By("Checking initial configuration")
		err := f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(vm.Status.Resources.CPU.Cores).To(Equal(initialCPUCores))
		Expect(vm.Status.Resources.Memory.Size).To(Equal(resource.MustParse(initialMemorySize)))

		By("Applying changes")
		err = f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)
		Expect(err).NotTo(HaveOccurred())
		runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, vm.Status.Conditions)
		previousRunningTime := runningCondition.LastTransitionTime.Time

		vm.Spec.CPU.Cores = changedCPUCores
		vm.Spec.Memory.Size = resource.MustParse(changedMemorySize)
		err = f.Clients.GenericClient().Update(context.Background(), vm)
		Expect(err).NotTo(HaveOccurred())

		if restartApprovalMode == v1alpha2.Manual {
			util.RebootVirtualMachineBySSH(f, vm)
		}

		By("Waiting for VM to be rebooted")
		util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(vm), previousRunningTime, framework.LongTimeout)
		util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.ShortTimeout)

		By("Checking changed configuration")
		err = f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(vm), vm)
		Expect(err).NotTo(HaveOccurred())
		Expect(vm.Status.Resources.CPU.Cores).To(Equal(changedCPUCores))
		Expect(vm.Status.Resources.Memory.Size).To(Equal(resource.MustParse(changedMemorySize)))
	},
		Entry("when changes are applied manually", v1alpha2.Manual),
		Entry("when changes are applied automatically", v1alpha2.Automatic),
	)
})

func generateConfigurationResources(namespace string, restartApprovalMode v1alpha2.RestartApprovalMode) (vm *v1alpha2.VirtualMachine, vd *v1alpha2.VirtualDisk) {
	vd = vdbuilder.New(
		vdbuilder.WithName("vd"),
		vdbuilder.WithNamespace(namespace),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: object.ImageURLUbuntu,
		}),
	)

	vm = vmbuilder.New(
		vmbuilder.WithName("vm"),
		vmbuilder.WithNamespace(namespace),
		vmbuilder.WithCPU(1, ptr.To("5%")),
		vmbuilder.WithMemory(resource.MustParse(initialMemorySize)),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithProvisioningUserData(object.DefaultCloudInit),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: vd.Name,
			},
		),
		vmbuilder.WithRestartApprovalMode(restartApprovalMode),
	)

	return
}
