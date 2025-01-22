/*
Copyright 2024 Flant JSC

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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	. "github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const (
	InternalApiVersion = "internal.virtualization.deckhouse.io/v1"
	KubevirtVMIMKind   = "InternalVirtualizationVirtualMachineInstanceMigration"
)

func MigrateVirtualMachines(label map[string]string, templatePath string, virtualMachines ...string) {
	GinkgoHelper()
	migrationFilesPath := fmt.Sprintf("%s/migrations", templatePath)
	for _, vm := range virtualMachines {
		migrationFilePath := fmt.Sprintf("%s/%s.yaml", migrationFilesPath, vm)
		err := CreateMigrationManifest(vm, migrationFilePath, label)
		Expect(err).NotTo(HaveOccurred(), err)
		res := kubectl.Apply(kc.ApplyOptions{
			Filename:       []string{migrationFilePath},
			FilenameOption: kc.Filename,
			Namespace:      conf.Namespace,
		})
		Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
	}
}

func CreateMigrationManifest(vmName, filePath string, labels map[string]string) error {
	vmop := &virtv2.VirtualMachineOperation{
		TypeMeta: v1.TypeMeta{
			APIVersion: virtv2.SchemeGroupVersion.String(),
			Kind:       virtv2.VirtualMachineOperationKind,
		},
		ObjectMeta: v1.ObjectMeta{
			Name:   vmName,
			Labels: labels,
		},
		Spec: virtv2.VirtualMachineOperationSpec{
			Type:           virtv2.VMOPTypeEvict,
			VirtualMachine: vmName,
		},
	}
	err := WriteYamlObject(filePath, vmop)
	if err != nil {
		return err
	}

	return nil
}

var _ = Describe("Virtual machine migration", ginkgoutil.CommonE2ETestDecorators(), func() {
	testCaseLabel := map[string]string{"testcase": "vm-migration"}

	Context("Preparing the environment", func() {
		It("sets the namespace", func() {
			kustomization := fmt.Sprintf("%s/%s", conf.TestData.VmMigration, "kustomization.yaml")
			ns, err := kustomize.GetNamespace(kustomization)
			Expect(err).NotTo(HaveOccurred(), "%w", err)
			conf.SetNamespace(ns)
		})
	})

	Context("When resources are applied", func() {
		It("result should be succeeded", func() {
			if config.IsReusable() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				if res.StdOut() != "" {
					return
				}
			}

			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.VmMigration},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual images are applied", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVI, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual disks are applied", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied", func() {
		It("checks VMs phases", func() {
			By(fmt.Sprintf("VMs should be in %s phases", PhaseRunning))
			WaitPhaseByLabel(kc.ResourceVM, PhaseRunning, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context(fmt.Sprintf("When virtual machines are in %s phases", PhaseRunning), func() {
		It("starts migrations", func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			MigrateVirtualMachines(testCaseLabel, conf.TestData.VmMigration, vms...)
		})
	})

	Context("When VMs migrations are applied", func() {
		It("checks VMs and VMOPs phases", func() {
			By(fmt.Sprintf("VMOPs should be in %s phases", virtv2.VMOPPhaseCompleted))
			WaitPhaseByLabel(kc.ResourceVMOP, string(virtv2.VMOPPhaseCompleted), kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
			By("Virtual machines should be migrated")
			WaitByLabel(kc.ResourceVM, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
				For:       "'jsonpath={.status.migrationState.result}=Succeeded'",
			})
		})

		It("checks VMs external connection after migrations", func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			CheckExternalConnection(externalHost, httpStatusOk, vms...)
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			resourcesToDelete := ResourcesToDelete{
				AdditionalResources: []AdditionalResource{
					{
						Resource: kc.ResourceVMOP,
						Labels:   testCaseLabel,
					},
				},
			}

			if !config.IsCleanUpNeeded() {
				resourcesToDelete.KustomizationDir = conf.TestData.VmMigration
			}

			DeleteTestCaseResources(resourcesToDelete)
		})
	})
})
