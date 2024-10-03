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

	. "github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const (
	InternalApiVersion = "internal.virtualization.deckhouse.io/v1"
	KubevirtVMIMKind   = "InternalVirtualizationVirtualMachineInstanceMigration"
)

var MigrationLabel = map[string]string{"testcase": "vm-migration"}

func MigrateVirtualMachines(virtualMachines ...string) {
	GinkgoHelper()
	migrationFilesPath := fmt.Sprintf("%s/migrations", conf.TestData.VmMigration)
	for _, vm := range virtualMachines {
		migrationFilePath := fmt.Sprintf("%s/%s.yaml", migrationFilesPath, vm)
		err := CreateMigrationManifest(vm, migrationFilePath, MigrationLabel)
		Expect(err).NotTo(HaveOccurred(), err)
		res := kubectl.Apply(migrationFilePath, kc.ApplyOptions{
			Namespace: conf.Namespace,
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

var _ = Describe("Virtual machine migration", Ordered, ContinueOnFailure, func() {
	Context("When resources are applied:", func() {
		It("must have no errors", func() {
			res := kubectl.Kustomize(conf.TestData.VmMigration, kc.KustomizeOptions{})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual disks are applied:", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be in %s phases", PhaseReady))
			WaitPhase(kc.ResourceVD, PhaseReady, kc.GetOptions{
				Labels:    MigrationLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It("checks VMs phases", func() {
			By(fmt.Sprintf("VMs should be in %s phases", PhaseRunning))
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
		It("checks VMs and KubevirtVMIMs phases", func() {
			By(fmt.Sprintf("KubevirtVMIMs should be in %s phases", PhaseSucceeded))
			WaitPhase(kc.ResourceKubevirtVMIM, PhaseSucceeded, kc.GetOptions{
				Labels:    MigrationLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			By(fmt.Sprintf("Virtual machines should be in %s phase", PhaseRunning))
			WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
				Labels:    MigrationLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})

		It("checks VMs external connection after migrations", func() {
			res := kubectl.List(kc.ResourceVM, kc.GetOptions{
				Labels:    MigrationLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())

			vms := strings.Split(res.StdOut(), " ")
			CheckExternalConnection(externalHost, httpStatusOk, vms...)
		})
	})
})
