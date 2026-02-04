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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
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

	It("checks if a VirtualMachine can be migrated to the target Node", func() {
		By("Environment preparation", func() {
			virtualDisk := object.NewHTTPVDAlpineBIOS(
				"vd-root",
				f.Namespace().Name,
			)

			virtualMachine = object.NewMinimalVM(
				"vm-",
				f.Namespace().Name,
				vm.WithBootloader(v1alpha2.BIOS),
				vm.WithDisks(virtualDisk),
				vm.WithTolerations([]corev1.Toleration{
					{
						Key:      "node-role.kubernetes.io/control-plane",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				}),
			)

			err := f.CreateWithDeferredDeletion(context.Background(), virtualDisk, virtualMachine)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, virtualMachine)
		})

		By("Migrate the `VirtualMachine`", func() {
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

			intvirtvmim, err := getVirtualMachineInstanceMigration(f, fmt.Sprintf("vmop-%s", targetMigrationVMOP.Name))
			Expect(err).NotTo(HaveOccurred())
			Expect(intvirtvmim).NotTo(BeNil())
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
	err := f.Clients.GenericClient().List(
		context.Background(),
		nodes,
		client.MatchingLabels(
			map[string]string{
				"virtualization.deckhouse.io/kvm-enabled": "true",
			},
		),
	)
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

func getVirtualMachineInstanceMigration(f *framework.Framework, name string) (*virtv1.VirtualMachineInstanceMigration, error) {
	obj := &rewrite.VirtualMachineInstanceMigration{}
	err := f.RewriteClient().Get(context.Background(), name, obj, rewrite.InNamespace(f.Namespace().Name))
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return obj.VirtualMachineInstanceMigration, nil
}
