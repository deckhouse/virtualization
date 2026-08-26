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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// The Migratable condition answers two questions at once: whether the machine itself is fit for a
// live migration, and whether the cluster has a node to move it to. The second half is verified
// here by pinning the machine to the node it already runs on: no other node matches the selector,
// so the cluster has nowhere to move it, while the machine itself stays perfectly migratable.
var _ = Describe("VirtualMachineMigratable", Label(label.SIGCompute, precheck.PrecheckMigratable), func() {
	var (
		f *framework.Framework
		t *migratableTest
	)

	BeforeEach(func() {
		f = framework.NewFramework("vm-migratable")
		DeferCleanup(f.After)
		f.Before()
		t = &migratableTest{Framework: f}
	})

	It("follows the cluster losing and regaining a migration target", func() {
		t.reportsMissingMigrationTarget()
	})

	It("drops the condition while the machine is not running", func() {
		t.dropsConditionWhileStopped()
	})
})

type migratableTest struct {
	Framework *framework.Framework

	VM *v1alpha2.VirtualMachine
	VD *v1alpha2.VirtualDisk
}

func (t *migratableTest) reportsMissingMigrationTarget() {
	ctx := context.Background()

	By("Environment preparation")
	t.generateResources("vm-migratable")
	Expect(t.Framework.CreateWithDeferredDeletion(ctx, t.VD, t.VM)).To(Succeed())
	vmObs := vmobs.StartObserver(ctx, t.Framework, t.VM)

	By("Waiting for the virtual machine to run")
	Expect(vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
	Expect(vmObs.WaitFor(haveMigratableReason(vmcondition.ReasonMigratable), framework.MiddleTimeout)).To(Succeed())

	node, err := util.GetVMNode(ctx, t.Framework, t.VM)
	Expect(err).NotTo(HaveOccurred())

	By("Pinning the virtual machine to the node it already runs on")
	t.patchNodeSelector(ctx, map[string]string{"kubernetes.io/hostname": node})

	By("Checking the virtual machine reports that it has nowhere to migrate")
	Expect(vmObs.WaitFor(haveMigratableReason(vmcondition.ReasonNoMigrationTarget), framework.MiddleTimeout)).To(Succeed())

	By("Removing the pin")
	t.patchNodeSelector(ctx, nil)

	// A machine that lost its only target must report the target back once the cluster has one
	// again, without waiting for anything else to happen to the machine.
	By("Checking the virtual machine becomes migratable again")
	Expect(vmObs.WaitFor(haveMigratableReason(vmcondition.ReasonMigratable), framework.MiddleTimeout)).To(Succeed())
}

func (t *migratableTest) dropsConditionWhileStopped() {
	ctx := context.Background()

	By("Environment preparation")
	t.generateResources("vm-migratable-stopped")
	Expect(t.Framework.CreateWithDeferredDeletion(ctx, t.VD, t.VM)).To(Succeed())
	vmObs := vmobs.StartObserver(ctx, t.Framework, t.VM)

	By("Waiting for the virtual machine to run")
	Expect(vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
	Expect(vmObs.WaitFor(haveMigratableCondition(true), framework.MiddleTimeout)).To(Succeed())

	By("Stopping the virtual machine")
	vmop := vmopbuilder.New(
		vmopbuilder.WithName(fmt.Sprintf("%s-migratable-stop", util.VmopE2ePrefix)),
		vmopbuilder.WithNamespace(t.VM.Namespace),
		vmopbuilder.WithType(v1alpha2.VMOPTypeStop),
		vmopbuilder.WithVirtualMachine(t.VM.Name),
	)
	Expect(t.Framework.CreateWithDeferredDeletion(ctx, vmop)).To(Succeed())
	Expect(vmObs.WaitFor(vmobs.BeStopped(), framework.LongTimeout)).To(Succeed())

	// Migratability describes a running machine: with no instance to look at, the previous answer
	// would be reported as the current one long after it stopped being true.
	By("Checking the condition is gone while the machine is stopped")
	Expect(vmObs.WaitFor(haveMigratableCondition(false), framework.MiddleTimeout)).To(Succeed())
}

func (t *migratableTest) generateResources(vmName string) {
	t.VD = object.NewVDFromCVI(fmt.Sprintf("vd-%s", vmName), t.Framework.Namespace().Name, object.PrecreatedCVICustomBIOS)
	t.VM = object.NewMinimalVM("", t.Framework.Namespace().Name,
		vmbuilder.WithName(vmName),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: t.VD.Name,
		}),
	)
}

func (t *migratableTest) patchNodeSelector(ctx context.Context, selector map[string]string) {
	GinkgoHelper()

	value := any(selector)
	if selector == nil {
		value = map[string]string{}
	}
	patch, err := json.Marshal([]map[string]any{{
		"op":    "replace",
		"path":  "/spec/nodeSelector",
		"value": value,
	}})
	Expect(err).NotTo(HaveOccurred())
	Expect(t.Framework.GenericClient().Patch(ctx, t.VM, crclient.RawPatch(types.JSONPatchType, patch))).To(Succeed())
}

// haveMigratableReason reports the Migratable condition carries the given reason.
func haveMigratableReason(reason vmcondition.MigratableReason) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond := meta.FindStatusCondition(vm.Status.Conditions, vmcondition.TypeMigratable.String())
		if cond == nil {
			return false, nil
		}
		return cond.Reason == reason.String(), nil
	}
}

// haveMigratableCondition reports whether the Migratable condition is present at all.
func haveMigratableCondition(present bool) vmobs.Predicate {
	return func(vm *v1alpha2.VirtualMachine) (bool, error) {
		cond := meta.FindStatusCondition(vm.Status.Conditions, vmcondition.TypeMigratable.String())
		return (cond != nil) == present, nil
	}
}
