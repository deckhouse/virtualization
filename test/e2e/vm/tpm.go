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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VMCheckTPM", func() {
	f := framework.NewFramework("vm-tpm-check")

	BeforeEach(func() {
		DeferCleanup(f.After)

		f.Before()
	})

	It("checks that TPM exists in the VM", func() {
		By("Create a VM with the TPM module.")
		const (
			expectedTPMVersion = "2.0"
			imageURLDebian12   = "https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru/debian/debian-12-with-tpm2-tools-amd64-20250814-2204.qcow2"
			vdsize             = "4.4Gi"
			bootLoader         = "EFI"
			osType             = "Windows"
			cloudInit          = `#cloud-config
ssh_pwauth: True
users:
  - name: cloud
    # passwd: cloud
    passwd: "$6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i."
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    lock_passwd: False
    ssh_authorized_keys:
      - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFxcXHmwaGnJ8scJaEN5RzklBPZpVSic4GdaAsKjQoeA your_email@example.com
runcmd:
  - systemctl enable qemu-guest-agent --now
  - chown -R cloud:cloud /home/cloud
`
		)

		vdRoot := vd.New(
			vd.WithName("vd-root"),
			vd.WithSize(ptr.To(resource.MustParse(vdsize))),
			vd.WithNamespace(f.Namespace().Name),
			vd.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
				URL: imageURLDebian12,
			}),
		)
		vmTPM := vm.New(
			vm.WithName("vm-with-tpm"),
			vm.WithNamespace(f.Namespace().Name),
			vm.WithCPU(1, ptr.To("100%")),
			vm.WithMemory(resource.MustParse("512Mi")),
			vm.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
			vm.WithDisks(vdRoot),
			vm.WithBootloader(bootLoader),
			vm.WithOsType(osType),
			vm.WithProvisioningUserData(cloudInit),
		)
		err := f.CreateWithDeferredDeletion(context.Background(), vdRoot, vmTPM)
		Expect(err).NotTo(HaveOccurred())
		util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, vmTPM)
		util.UntilSSHReady(f, vmTPM, framework.LongTimeout)

		By(fmt.Sprintf("Checks that the VM has the TPM module version %s.", expectedTPMVersion))
		cmd := `sudo tpm2_getcap properties-fixed | grep -A2 TPM2_PT_FAMILY_INDICATOR | grep value`
		Eventually(func() error {
			cmdStdOut, err := f.SSHCommand(vmTPM.Name, vmTPM.Namespace, cmd)
			if err != nil {
				return err
			}

			Expect(cmdStdOut).To(ContainSubstring(expectedTPMVersion))

			return nil
		}).WithTimeout(framework.ShortTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
	})
})
