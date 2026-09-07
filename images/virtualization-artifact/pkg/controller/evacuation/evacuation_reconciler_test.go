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

package evacuation

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestEvacuationReconciler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Evacuation Reconciler Suite")
}

var _ = Describe("Reconcile with absent VirtualMachine", func() {
	const (
		vmName      = "vm-evacuate"
		vmNamespace = "default"
	)

	ctx := testutil.ContextBackgroundWithNoOpLogger()

	newVMOP := func(name, vmName string) *v1alpha2.VirtualMachineOperation {
		return vmopbuilder.New(
			vmopbuilder.WithName(name),
			vmopbuilder.WithNamespace(vmNamespace),
			vmopbuilder.WithFinalizer(v1alpha2.FinalizerVMOPProtectionByEvacuationController),
			vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
			vmopbuilder.WithVirtualMachine(vmName),
		)
	}

	It("Should release the protection finalizer from orphaned VMOPs only", func() {
		orphaned := newVMOP("evacuation-orphaned", vmName)
		foreign := newVMOP("evacuation-foreign", "other-vm")

		fakeClient, err := testutil.NewFakeClientWithObjects(orphaned, foreign)
		Expect(err).NotTo(HaveOccurred())

		r := NewReconciler(fakeClient, nil)
		_, err = r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: vmName, Namespace: vmNamespace},
		})
		Expect(err).NotTo(HaveOccurred())

		got := &v1alpha2.VirtualMachineOperation{}
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(orphaned), got)).To(Succeed())
		Expect(got.GetFinalizers()).NotTo(ContainElement(v1alpha2.FinalizerVMOPProtectionByEvacuationController))

		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(foreign), got)).To(Succeed())
		Expect(got.GetFinalizers()).To(ContainElement(v1alpha2.FinalizerVMOPProtectionByEvacuationController))
	})
})
