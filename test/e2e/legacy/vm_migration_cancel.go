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

package legacy

import (
	"fmt"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/d8"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	kc "github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
)

var _ = Describe("VirtualMachineCancelMigration", Label("legacy"), Ordered, func() {
	testCaseLabel := map[string]string{"testcase": "vm-migration-cancel"}
	var ns string

	BeforeAll(func() {
		// TODO: Remove Skip after fixing the issue.
		Skip("This test case is not working everytime. Should be fixed.")

		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VMMigrationCancel, "kustomization.yaml")
		var err error
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)

		CreateNamespace(ns)
	})

	BeforeEach(func() {
		res := kubectl.Apply(kc.ApplyOptions{
			Filename:       []string{conf.TestData.VMMigrationCancel},
			FilenameOption: kc.Kustomize,
		})
		Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestCaseDump(testCaseLabel, CurrentSpecReport().LeafNodeText, ns)
		}
		resourcesToDelete := ResourcesToDelete{
			AdditionalResources: []AdditionalResource{
				{
					Resource: kc.ResourceVMOP,
					Labels:   testCaseLabel,
				},
			},
		}

		if config.IsCleanUpNeeded() {
			resourcesToDelete.KustomizationDir = conf.TestData.VMMigrationCancel
		}
		DeleteTestCaseResources(ns, resourcesToDelete)
	})

	It("Cancel migrate", func() {
		By("Virtual machine agents should be ready")
		WaitVMAgentReady(kc.WaitOptions{
			Labels:    testCaseLabel,
			Namespace: ns,
			Timeout:   MaxWaitTimeout,
		})

		vms := &v1alpha2.VirtualMachineList{}
		err := GetObjects(kc.ResourceVM, vms, kc.GetOptions{Labels: testCaseLabel, Namespace: ns})
		Expect(err).NotTo(HaveOccurred())

		vmNames := make([]string, len(vms.Items))
		for i, vm := range vms.Items {
			vmNames[i] = vm.GetName()
		}

		for _, name := range vmNames {
			By(fmt.Sprintf("Exec SSHCommand for virtualmachine %s/%s", ns, name))
			res := framework.GetClients().D8Virtualization().SSHCommand(name, "sudo nohup stress-ng --vm 1 --vm-bytes 100% --timeout 300s &>/dev/null &", d8.SSHOptions{
				Namespace:    ns,
				Username:     conf.TestData.SSHUser,
				IdentityFile: conf.TestData.Sshkey,
				Timeout:      ShortTimeout,
			})
			Expect(res.WasSuccess()).To(BeTrue(), res.StdErr())
		}

		By("Wait until stress-ng loads the memory more heavily")
		time.Sleep(20 * time.Second)

		By("Starting migrations for virtual machines")
		MigrateVirtualMachines(testCaseLabel, ns, vmNames...)

		someCompleted := false

		Eventually(func() error {
			vmops := &v1alpha2.VirtualMachineOperationList{}
			err := GetObjects(kc.ResourceVMOP, vmops, kc.GetOptions{Labels: testCaseLabel, Namespace: ns})
			if err != nil {
				return err
			}

			if len(vmops.Items) == 0 {
				return nil
			}

			kvvmis := &virtv1.VirtualMachineInstanceList{}
			err = GetObjects(kc.ResourceKubevirtVMI, kvvmis, kc.GetOptions{Labels: testCaseLabel, Namespace: ns})
			if err != nil {
				return err
			}

			kvvmisByName := make(map[string]*virtv1.VirtualMachineInstance, len(kvvmis.Items))
			for _, kvvmi := range kvvmis.Items {
				kvvmisByName[kvvmi.Name] = &kvvmi
			}

			migrationReady := make(map[string]struct{})
			for _, vmop := range vmops.Items {
				if kvvmi := kvvmisByName[vmop.Spec.VirtualMachine]; kvvmi != nil {
					if kvvmi.Status.MigrationState != nil && !kvvmi.Status.MigrationState.StartTimestamp.IsZero() {
						migrationReady[vmop.Name] = struct{}{}
					}
				}
			}

			for _, vmop := range vmops.Items {
				switch vmop.Status.Phase {
				case v1alpha2.VMOPPhaseInProgress:
					_, readyToDelete := migrationReady[vmop.Name]

					if readyToDelete && vmop.GetDeletionTimestamp().IsZero() {
						res := kubectl.Delete(kc.DeleteOptions{
							Resource:  kc.ResourceVMOP,
							Name:      vmop.GetName(),
							Namespace: vmop.GetNamespace(),
						})
						if !res.WasSuccess() {
							return res.Error()
						}
					}
				case v1alpha2.VMOPPhaseFailed, v1alpha2.VMOPPhaseCompleted:
					someCompleted = true
					return nil
				}
			}
			return fmt.Errorf("retry because not all vmops canceled")
		}).WithTimeout(MaxWaitTimeout).WithPolling(time.Second).ShouldNot(HaveOccurred())

		Expect(someCompleted).Should(BeFalse())

		By("Abort status should be exists in Kubevirt VMIs")

		validAbortStatuses := []virtv1.MigrationAbortStatus{
			virtv1.MigrationAbortInProgress,
			virtv1.MigrationAbortSucceeded,
			virtv1.MigrationAbortFailed,
		}

		Eventually(func() error {
			kvvmis := &virtv1.VirtualMachineInstanceList{}
			err = GetObjects(kc.ResourceKubevirtVMI, kvvmis, kc.GetOptions{Labels: testCaseLabel, Namespace: ns})
			if err != nil {
				return err
			}
			for _, kvvmi := range kvvmis.Items {
				migrationState := kvvmi.Status.MigrationState

				if migrationState == nil {
					return fmt.Errorf("retry because migration state is nil")
				}
				if !migrationState.AbortRequested {
					return fmt.Errorf("retry because migration abort requested is false")
				}

				if !slices.Contains(validAbortStatuses, migrationState.AbortStatus) {
					return fmt.Errorf("retry because migration abort status is %s", migrationState.AbortStatus)
				}

				if migrationState.EndTimestamp.IsZero() {
					return fmt.Errorf("retry because migration is not finished yet")
				}
			}
			return nil
		}).WithTimeout(LongWaitDuration).WithPolling(time.Second).ShouldNot(HaveOccurred())
	})
})
