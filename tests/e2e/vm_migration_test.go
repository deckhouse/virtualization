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
	virtv1 "kubevirt.io/api/core/v1"

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
	vmim := &virtv1.VirtualMachineInstanceMigration{
		TypeMeta: v1.TypeMeta{
			APIVersion: InternalApiVersion,
			Kind:       KubevirtVMIMKind,
		},
		ObjectMeta: v1.ObjectMeta{
			Name:   vmName,
			Labels: labels,
		},
		Spec: virtv1.VirtualMachineInstanceMigrationSpec{
			VMIName: vmName,
		},
	}

	err := WriteYamlObject(filePath, vmim)
	if err != nil {
		return err
	}

	return nil
}

var _ = Describe("Virtual machine migration", ginkgoutil.CommonE2ETestDecorators(), func() {
	testCaseLabel := map[string]string{"testcase": "vm-migration"}

	Context("When resources are applied:", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.VmMigration},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual images are applied:", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVI, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual disks are applied:", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It("checks VMs phases", func() {
			By(fmt.Sprintf("VMs should be in %s phases", PhaseRunning))
			WaitPhaseByLabel(kc.ResourceVM, PhaseRunning, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context(fmt.Sprintf("When virtual machines are in %s phases:", PhaseRunning), func() {
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

	Context("When VMs migrations are applied:", func() {
		It("checks VMs and KubevirtVMIMs phases", func() {
			By(fmt.Sprintf("KubevirtVMIMs should be in %s phases", PhaseSucceeded))
			WaitPhaseByLabel(kc.ResourceKubevirtVMIM, PhaseSucceeded, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
			By(fmt.Sprintf("Virtual machines should be in %s phase", PhaseRunning))
			WaitPhaseByLabel(kc.ResourceVM, PhaseRunning, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
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

	Context("When test is complited:", func() {
		It("tries to delete used resources", func() {
			kustimizationFile := fmt.Sprintf("%s/%s", conf.TestData.VmMigration, "kustomization.yaml")
			err := kustomize.ExcludeResource(kustimizationFile, "ns.yaml")
			Expect(err).NotTo(HaveOccurred(), "cannot exclude namespace from clean up operation:\n%s", err)
			res := kubectl.Delete(kc.DeleteOptions{
				Filename:       []string{conf.TestData.VmMigration},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
			res = kubectl.Delete(kc.DeleteOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Resource:  kc.ResourceKubevirtVMIM,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), "cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		})
	})
})
