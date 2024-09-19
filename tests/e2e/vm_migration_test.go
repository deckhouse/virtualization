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
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	yamlv3 "gopkg.in/yaml.v3"

	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	. "github.com/deckhouse/virtualization/tests/e2e/resources"
)

var MigrationLabel = map[string]string{"testcase": "vm-migration"}

func MigrateVirtualMachines(virtualMachines ...string) {
	GinkgoHelper()
	migrationFilesPath := fmt.Sprintf("%s/migrations", conf.TestData.VmMigration)
	templatePath := fmt.Sprintf("%s/vm-migration.yaml", migrationFilesPath)
	template, err := GetVirtualMachineMigrationManifest(templatePath)
	Expect(err).NotTo(HaveOccurred(), err)
	for _, vm := range virtualMachines {
		migrationFilePath := fmt.Sprintf("%s/%s.yaml", migrationFilesPath, vm)
		createErr := CreateMigrationManifest(vm, migrationFilePath, MigrationLabel, template)
		Expect(createErr).NotTo(HaveOccurred(), createErr)
		applyRes := kubectl.Apply(migrationFilePath, kc.ApplyOptions{
			Namespace: conf.Namespace,
		})
		Expect(applyRes.Error()).NotTo(HaveOccurred(), applyRes.StdErr())
	}
}

func CreateMigrationManifest(vmName, filePath string, labels map[string]string, template *VirtualMachineMigration) error {
	template.Metadata.Name = vmName
	template.Spec.VmiName = vmName
	template.Metadata.Labels = labels
	data, marshalErr := yamlv3.Marshal(template)
	if marshalErr != nil {
		return marshalErr
	}
	writeErr := os.WriteFile(filePath, data, 0o644)
	if writeErr != nil {
		return writeErr
	}
	return nil
}

var _ = Describe("Virtual machine migration", Ordered, ContinueOnFailure, func() {
	Context("When resources are applied:", func() {
		It("must has no errors", func() {
			res := kubectl.Kustomize(conf.TestData.VmMigration, kc.KustomizeOptions{})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual disks are applied:", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be are in %s phases", PhaseReady))
			WaitPhase(kc.ResourceVD, PhaseReady, kc.GetOptions{
				Labels:    MigrationLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It("checks VMs phases", func() {
			By(fmt.Sprintf("VMs should be are in %s phases", PhaseRunning))
			WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
				Labels:    MigrationLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context(fmt.Sprintf("When virtual machines are in %s phases:", PhaseRunning), func() {
		It("starts migrations", func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    MigrationLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			MigrateVirtualMachines(vms...)
		})
	})

	Context("When VMs migrations are applied:", func() {
		It("checks VMs and INTVIRTVMIMs phases", func() {
			By(fmt.Sprintf("INTVIRTVMIMs should be are in %s phases", PhaseSucceeded))
			WaitPhase(kc.ResourceKubevirtVMIM, PhaseSucceeded, kc.GetOptions{
				Labels:    MigrationLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			By(fmt.Sprintf("Virtual machines should be are in %s phase", PhaseRunning))
			WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
				Labels:    MigrationLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})

		It("checks VMs external connection after migrations", func() {
			sshKeyPath := fmt.Sprintf("%s/id_ed", conf.TestData.Sshkeys)

			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    MigrationLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			CheckExternalConnection(sshKeyPath, externalHost, httpStatusOk, vms...)
		})
	})
})
