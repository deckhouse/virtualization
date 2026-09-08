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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// The SPICE display is built into the domain when it starts, so both turning it on and
// turning it off go through a restart. Turning it off is the half that used to do
// nothing: the builder works on top of the KVVM that already exists, so devices left
// behind outlived the restart and the display stayed up along with the memory reserved
// for it.
var _ = Describe("VirtualMachineSPICE", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("vm-spice")
		DeferCleanup(f.After)
		f.Before()
	})

	It("attaches the SPICE devices when enabled and detaches them when disabled", func() {
		By("Environment preparation")
		vd := object.NewVDFromCVI("vd-vm-spice", f.Namespace().Name, object.PrecreatedCVICustomBIOS)
		testVM := object.NewMinimalVM("", f.Namespace().Name,
			vmbuilder.WithName("vm-spice"),
			vmbuilder.WithSpice(true),
			// The restart the change asks for is done by the platform, so the spec is the
			// only thing this test touches.
			vmbuilder.WithRestartApprovalMode(v1alpha2.Automatic),
			vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: vd.Name,
			}),
		)
		err := f.CreateWithDeferredDeletion(ctx, vd, testVM)
		Expect(err).NotTo(HaveOccurred())
		vmObs := vmobs.StartObserver(ctx, f, testVM)
		vmObs.Never(vmobs.BeFailed())
		Expect(vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())

		By("Checking the running VMI carries the SPICE devices")
		Eventually(func(g Gomega) {
			kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, testVM)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(kvvmi).NotTo(BeNil())
			g.Expect(kvvmi.Annotations).To(HaveKeyWithValue(annotations.AnnSpice, "true"))
			g.Expect(kvvmi.Spec.Domain.Devices.Video).NotTo(BeNil())
			g.Expect(kvvmi.Spec.Domain.Devices.Sound).NotTo(BeNil())
			g.Expect(kvvmi.Spec.Domain.Devices.ClientPassthrough).NotTo(BeNil())
		}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed())

		By("Disabling SPICE")
		patch, err := json.Marshal([]map[string]interface{}{{
			"op":    "replace",
			"path":  "/spec/spice/enabled",
			"value": false,
		}})
		Expect(err).NotTo(HaveOccurred())
		Expect(f.GenericClient().Patch(ctx, testVM, crclient.RawPatch(types.JSONPatchType, patch))).To(Succeed())

		By("Waiting until the restart has applied it")
		Eventually(func(g Gomega) {
			kvvmi, err := util.GetInternalVirtualMachineInstance(ctx, testVM)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(kvvmi).NotTo(BeNil())
			g.Expect(kvvmi.Status.Phase).To(Equal(virtv1.Running))
			g.Expect(kvvmi.Annotations).NotTo(HaveKey(annotations.AnnSpice))
			g.Expect(kvvmi.Spec.Domain.Devices.Video).To(BeNil())
			g.Expect(kvvmi.Spec.Domain.Devices.Sound).To(BeNil())
			g.Expect(kvvmi.Spec.Domain.Devices.ClientPassthrough).To(BeNil())
		}).WithTimeout(framework.LongTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
	})
})
