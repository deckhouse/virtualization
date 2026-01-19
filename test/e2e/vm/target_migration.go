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

package vm

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const hostnameLabelKey = "kubernetes.io/hostname"

var _ = Describe("TargetMigration", func() {
	var (
		virtualMachine      *v1alpha2.VirtualMachine
		targetMigrationVMOP *v1alpha2.VirtualMachineOperation

		initialNodeName    string
		targetNodeSelector map[string]string

		f = framework.NewFramework("vm-target-migration")
	)

	BeforeEach(func() {
		DeferCleanup(f.After)
		f.Before()
	})

	It("checks a `VirtualMachine` migrate to the target `Node`", func() {
		By("Environment preparation", func() {
			checkNodeSelectorRequirements(f)

			virtaulDisk := vd.New(
				vd.WithName("vd"),
				vd.WithNamespace(f.Namespace().Name),
				vd.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
					URL: object.ImageURLAlpineUEFIPerf,
				}),
			)

			virtualMachine = vm.New(
				vm.WithName("vm"),
				vm.WithNamespace(f.Namespace().Name),
				vm.WithBootloader(v1alpha2.EFI),
				vm.WithCPU(1, ptr.To("10%")),
				vm.WithMemory(*resource.NewQuantity(object.Mi256, resource.BinarySI)),
				vm.WithDisks(virtaulDisk),
				vm.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
				vm.WithProvisioning(&v1alpha2.Provisioning{
					Type:     v1alpha2.ProvisioningTypeUserData,
					UserData: object.DefaultCloudInit,
				}),
				vm.WithTolerations([]corev1.Toleration{
					{
						Key:      "node-role.kubernetes.io/control-plane",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				}),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), virtaulDisk, virtualMachine)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, virtualMachine)
		})

		By("Migrate a `VirtualMachine`", func() {
			virtualMachine, err := f.Clients.VirtClient().VirtualMachines(f.Namespace().Name).Get(context.Background(), virtualMachine.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			initialNodeName = virtualMachine.Status.Node
			targetNodeSelector, err = defineTargetNodeSelector(f, initialNodeName)
			Expect(err).NotTo(HaveOccurred())

			targetMigrationVMOP = newTargetMigrationVMOP(virtualMachine, targetNodeSelector)
			err = f.CreateWithDeferredDeletion(context.Background(), targetMigrationVMOP)
			Expect(err).NotTo(HaveOccurred())

			util.UntilVMMigrationSucceeded(client.ObjectKeyFromObject(virtualMachine), framework.MaxTimeout)
			util.UntilObjectPhase(string(v1alpha2.VMOPPhaseCompleted), framework.ShortTimeout, targetMigrationVMOP)
		})

		By("Check the result", func() {
			targetMigrationVMOP, err := f.Clients.VirtClient().VirtualMachineOperations(f.Namespace().Name).Get(context.Background(), targetMigrationVMOP.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			Expect(targetMigrationVMOP.Spec.Migrate).NotTo(BeNil())
			Expect(targetMigrationVMOP.Spec.Migrate.NodeSelector).To(HaveKey(hostnameLabelKey))

			intvirtvmim, err := getVirtualMachineInstanceMigration(targetMigrationVMOP)
			Expect(err).NotTo(HaveOccurred())
			Expect(intvirtvmim.Spec.AddedNodeSelector).To(HaveKey(hostnameLabelKey))
			Expect(intvirtvmim.Status.Phase).To(Equal(virtv1.MigrationSucceeded))

			virtualMachine, err := f.Clients.VirtClient().VirtualMachines(f.Namespace().Name).Get(context.Background(), virtualMachine.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			Expect(initialNodeName).NotTo(Equal(virtualMachine.Status.Node))
			Expect(virtualMachine.Status.Node).To(Equal(targetNodeSelector[hostnameLabelKey]))
		})
	})
})

func newTargetMigrationVMOP(virtualMachine *v1alpha2.VirtualMachine, nodeSelector map[string]string) *v1alpha2.VirtualMachineOperation {
	return vmopbuilder.New(
		vmopbuilder.WithGenerateName(fmt.Sprintf("%s-migrate-", util.VmopE2ePrefix)),
		vmopbuilder.WithNamespace(virtualMachine.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeMigrate),
		vmopbuilder.WithVirtualMachine(virtualMachine.Name),
		vmopbuilder.WithVMOPMigrateNodeSelector(nodeSelector),
	)
}

func defineTargetNodeSelector(f *framework.Framework, currentNodeName string) (map[string]string, error) {
	errMsg := "could not define a target node for the virtual machine"

	nodes := &corev1.NodeList{}
	err := f.Clients.GenericClient().List(context.Background(), nodes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}

	for _, node := range nodes.Items {
		if node.Name == currentNodeName {
			continue
		}

		if hostname, ok := node.Labels[hostnameLabelKey]; ok {
			if hostname != currentNodeName {
				return map[string]string{hostnameLabelKey: hostname}, nil
			}
		}
	}

	return nil, errors.New(errMsg)
}

// All nodes must have the `kubernetes.io/hostname` label, and its value must be equal to `node.Name`.
func checkNodeSelectorRequirements(f *framework.Framework) {
	GinkgoHelper()

	errMsg := "failed to check `NodeSelector` requirements"

	nodes := &corev1.NodeList{}
	err := f.Clients.GenericClient().List(context.Background(), nodes)
	Expect(err).NotTo(HaveOccurred(), errMsg)

	for _, node := range nodes.Items {
		Expect(node.Labels).To(HaveKey(hostnameLabelKey), errMsg)
		Expect(node.Name).To(Equal(node.Labels[hostnameLabelKey]), errMsg)
	}
}

func getVirtualMachineInstanceMigration(vmop *v1alpha2.VirtualMachineOperation) (*virtv1.VirtualMachineInstanceMigration, error) {
	vmimName := fmt.Sprintf("vmop-%s", vmop.Name)
	unstructuredVMIM, err := framework.GetClients().DynamicClient().Resource(schema.GroupVersionResource{
		Group:    "internal.virtualization.deckhouse.io",
		Version:  "v1",
		Resource: "internalvirtualizationvirtualmachineinstancemigrations",
	}).Namespace(vmop.Namespace).Get(context.Background(), vmimName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	var vmim virtv1.VirtualMachineInstanceMigration
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredVMIM.Object, &vmim)
	if err != nil {
		return nil, err
	}

	return &vmim, nil
}
