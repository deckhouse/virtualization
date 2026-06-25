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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	initialRunPolicy                = v1alpha2.AlwaysOnUnlessStoppedManually
	initialEnableParavirtualization = true
	changedRunPolicy                = v1alpha2.AlwaysOnPolicy
	changedEnableParavirtualization = false
)

var _ = Describe("VirtualMachineConfiguration", Label(precheck.NoPrecheck), func() {
	DescribeTable("the configuration should be applied", func(restartApprovalMode v1alpha2.RestartApprovalMode) {
		ctx := context.Background()
		f := framework.NewFramework(fmt.Sprintf("vm-configuration-%s", strings.ToLower(string(restartApprovalMode))))
		t := newConfigurationTest(f)

		DeferCleanup(f.After)
		f.Before()

		By("Environment preparation")
		t.GenerateResources(restartApprovalMode)
		err := f.CreateWithDeferredDeletion(ctx, t.VM, t.VDRoot)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VM will be running")
		util.UntilObjectPhase(ctx, "Running", framework.LongTimeout, t.VM)

		By("Checking initial configuration")
		err = f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
		Expect(err).NotTo(HaveOccurred())
		Expect(t.VM.Spec.RunPolicy).To(Equal(initialRunPolicy))
		Expect(t.VM.Spec.EnableParavirtualization).To(Equal(initialEnableParavirtualization))

		By("Applying changes")
		err = f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
		Expect(err).NotTo(HaveOccurred())
		runningCondition, _ := conditions.GetCondition(vmcondition.TypeRunning, t.VM.Status.Conditions)
		previousRunningTime := runningCondition.LastTransitionTime.Time

		By("Applying")
		patchset := patch.NewJSONPatch(
			patch.WithReplace("/spec/runPolicy", changedRunPolicy),
			patch.WithReplace("/spec/enableParavirtualization", changedEnableParavirtualization),
		)
		patchBytes, err := patchset.Bytes()
		Expect(err).NotTo(HaveOccurred())
		t.VM, err = f.VirtClient().VirtualMachines(t.VM.Namespace).Patch(ctx, t.VM.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		Expect(err).NotTo(HaveOccurred())

		if util.IsRestartRequired(t.VM, 10*time.Second) {
			util.RebootVirtualMachineByVMOP(f, t.VM)
		}

		By("Waiting for VM to be rebooted")
		util.UntilVirtualMachineRebooted(crclient.ObjectKeyFromObject(t.VM), previousRunningTime, framework.LongTimeout)
		util.UntilObjectPhase(ctx, "Running", framework.MiddleTimeout, t.VM)

		By("Checking changed configuration")
		err = f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
		Expect(err).NotTo(HaveOccurred())
		Expect(t.VM.Spec.RunPolicy).To(Equal(changedRunPolicy))
		Expect(t.VM.Spec.EnableParavirtualization).To(Equal(changedEnableParavirtualization))

		Consistently(func(g Gomega) {
			err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(t.VM), t.VM)
			g.Expect(err).NotTo(HaveOccurred())
			_, exists := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, t.VM.Status.Conditions)
			g.Expect(exists).To(BeFalse())
			g.Expect(t.VM.Status.RestartAwaitingChanges).To(BeNil())
		}).WithTimeout(10 * time.Second).WithPolling(time.Second).Should(Succeed())
	},
		Entry("when changes are applied manually", v1alpha2.Manual),
		Entry("when changes are applied automatically", v1alpha2.Automatic),
	)
})

type configurationTest struct {
	Framework *framework.Framework

	VM     *v1alpha2.VirtualMachine
	VDRoot *v1alpha2.VirtualDisk
}

func newConfigurationTest(f *framework.Framework) *configurationTest {
	return &configurationTest{
		Framework: f,
	}
}

func (t *configurationTest) GenerateResources(restartApprovalMode v1alpha2.RestartApprovalMode) {
	t.VDRoot = object.NewVDFromCVI("vd-root", t.Framework.Namespace().Name, object.PrecreatedCVIAlpineBIOS)

	t.VM = object.NewMinimalVM("vm", t.Framework.Namespace().Name,
		vmbuilder.WithEnableParavirtualization(ptr.To(initialEnableParavirtualization)),
		vmbuilder.WithRunPolicy(initialRunPolicy),
		vmbuilder.WithDisks(t.VDRoot),
		vmbuilder.WithRestartApprovalMode(restartApprovalMode),
	)
}
