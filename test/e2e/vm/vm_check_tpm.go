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
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

var _ = Describe("VMCheckTPM", label.TPM(), Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)
	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("vm-check-tpm")
		DeferCleanup(f.After)

		f.Before()
	})

	It("checks that TPM exists in the VM", func() {
		// Nested clusters expose no invariant TSC on their nodes, so this
		// Windows-osType VM would never start there.
		skipWithoutInvariantTSC(ctx, f)

		By("Create a VM with the TPM module.")
		const (
			expectedTPMVersion = "2.0"
			bootLoader         = "EFI"
			osType             = "Windows"
		)

		// The custom EFI image bakes in tpm2-tools (talking to
		// /dev/tpmrm0 directly) and the TPM kernel drivers, so the guest check
		// runs as root with no cloud-init provisioning.
		vdRoot := object.NewVDFromCVI("vd-root", f.Namespace().Name, object.PrecreatedCVICustomEFI,
			vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))),
		)
		vmTPM := vm.New(
			vm.WithName("vm-with-tpm"),
			vm.WithNamespace(f.Namespace().Name),
			vm.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
			vm.WithMemory(resource.MustParse(object.CustomImageEFIVMMemory)),
			vm.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
			vm.WithDisks(vdRoot),
			vm.WithBootloader(bootLoader),
			vm.WithOsType(osType),
		)
		err := f.CreateWithDeferredDeletion(ctx, vdRoot, vmTPM)
		Expect(err).NotTo(HaveOccurred())
		vmObs := vmobs.StartObserver(ctx, f, vmTPM)
		vmObs.Never(vmobs.BeFailed())
		err = vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
		eventually.SSHReadyAsRoot(f, vmTPM, framework.LongTimeout)

		By(fmt.Sprintf("Checks that the VM has the TPM module version %s.", expectedTPMVersion))
		cmd := `tpm2_getcap properties-fixed | grep -A2 TPM2_PT_FAMILY_INDICATOR | grep value`
		cmdStdOut, err := f.SSHCommand(vmTPM.Name, vmTPM.Namespace, cmd, framework.WithSSHUser("root"))
		Expect(err).NotTo(HaveOccurred())
		Expect(cmdStdOut).To(ContainSubstring(expectedTPMVersion))
	})
})
