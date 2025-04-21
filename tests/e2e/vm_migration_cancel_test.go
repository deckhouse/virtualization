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

package e2e

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	virtv1 "kubevirt.io/api/core/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var _ = Describe("Virtual machine migration cancel", SIGMigration(), ginkgoutil.CommonE2ETestDecorators(), func() {
	testCaseLabel := map[string]string{"testcase": "vm-migration-cancel"}

	BeforeEach(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.VmMigrationCancel, "kustomization.yaml")
		ns, err := kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)
		conf.SetNamespace(ns)

		res := kubectl.Apply(kc.ApplyOptions{
			Filename:       []string{conf.TestData.VmMigrationCancel},
			FilenameOption: kc.Kustomize,
		})
		Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
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
			resourcesToDelete.KustomizationDir = conf.TestData.VmMigrationCancel
		}
		DeleteTestCaseResources(resourcesToDelete)
	})

	It("Cancel migrate", func() {
		By("Virtual machine agents should be ready")
		WaitVmAgentReady(kc.WaitOptions{
			Labels:    testCaseLabel,
			Namespace: conf.Namespace,
			Timeout:   MaxWaitTimeout,
		})

		vms := &virtv2.VirtualMachineList{}
		err := GetObjects(kc.ResourceVM, vms, kc.GetOptions{Labels: testCaseLabel, Namespace: conf.Namespace})
		Expect(err).NotTo(HaveOccurred())

		vmNames := make([]string, len(vms.Items))
		for i, vm := range vms.Items {
			vmNames[i] = vm.GetName()
		}

		for _, name := range vmNames {
			By(fmt.Sprintf("Exec SSHCommand for virtualmachine %s/%s", conf.Namespace, name))
			res := d8Virtualization.SshCommand(name, "sudo nohup stress-ng --vm 1 --vm-bytes 100% --timeout 600s &>/dev/null &", d8.SshOptions{
				Namespace:   conf.Namespace,
				Username:    conf.TestData.SshUser,
				IdenityFile: conf.TestData.Sshkey,
				Timeout:     ShortTimeout,
			})
			Expect(res.WasSuccess()).To(BeTrue(), res.StdErr())
		}

		By("Wait until stress-ng loads the memory more heavily")
		time.Sleep(20 * time.Second)

		By("Starting migrations for virtual machines")
		MigrateVirtualMachines(testCaseLabel, vmNames...)

		someCompleted := false

		Eventually(func() error {
			vmops := &virtv2.VirtualMachineOperationList{}
			err := GetObjects(kc.ResourceVMOP, vmops, kc.GetOptions{Labels: testCaseLabel, Namespace: conf.Namespace})
			if err != nil {
				return err
			}

			if len(vmops.Items) == 0 {
				return nil
			}

			kvvmis := &virtv1.VirtualMachineInstanceList{}
			err = GetObjects(kc.ResourceKubevirtVMI, kvvmis, kc.GetOptions{Labels: testCaseLabel, Namespace: conf.Namespace})
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
				case virtv2.VMOPPhaseInProgress:
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
				case virtv2.VMOPPhaseFailed, virtv2.VMOPPhaseCompleted:
					someCompleted = true
					return nil
				}
			}
			return fmt.Errorf("retry because not all vmops canceled")

		}).WithTimeout(MaxWaitTimeout).WithPolling(time.Second).ShouldNot(HaveOccurred())

		Expect(someCompleted).Should(BeFalse())

		By("Abort status should be exists in Kubevirt VMIs")
		Eventually(func() error {
			kvvmis := &virtv1.VirtualMachineInstanceList{}
			err = GetObjects(kc.ResourceKubevirtVMI, kvvmis, kc.GetOptions{Labels: testCaseLabel, Namespace: conf.Namespace})
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
				validAbortStatus := migrationState.AbortStatus == virtv1.MigrationAbortFailed || migrationState.AbortStatus == virtv1.MigrationAbortSucceeded
				if !validAbortStatus {
					return fmt.Errorf("retry because migration abort status is %s", migrationState.AbortStatus)
				}
			}
			return nil
		}).WithTimeout(LongWaitDuration).WithPolling(time.Second).ShouldNot(HaveOccurred())
	})
})
