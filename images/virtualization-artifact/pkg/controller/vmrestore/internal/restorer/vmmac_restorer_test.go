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

package restorer

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("VirtualMachineMACAddressOverrideValidator.ValidateWithForce", func() {
	const (
		namespace = "default"
		vmmacName = "vm-ub24-2wxqs"
		restoreVM = "vm-ub24"
		address   = "be:5e:2b:e6:6f:a1"
	)

	var scheme *runtime.Scheme

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
	})

	// template is the vmmac captured in the snapshot: it knows which VM it belonged to.
	newTemplate := func() *v1alpha2.VirtualMachineMACAddress {
		return &v1alpha2.VirtualMachineMACAddress{
			ObjectMeta: metav1.ObjectMeta{Name: vmmacName, Namespace: namespace},
			Spec:       v1alpha2.VirtualMachineMACAddressSpec{Address: address},
			Status: v1alpha2.VirtualMachineMACAddressStatus{
				Address:        address,
				VirtualMachine: restoreVM,
			},
		}
	}

	existing := func(phase v1alpha2.VirtualMachineMACAddressPhase, vm string) *v1alpha2.VirtualMachineMACAddress {
		return &v1alpha2.VirtualMachineMACAddress{
			ObjectMeta: metav1.ObjectMeta{Name: vmmacName, Namespace: namespace},
			Spec:       v1alpha2.VirtualMachineMACAddressSpec{Address: address},
			Status: v1alpha2.VirtualMachineMACAddressStatus{
				Address:        address,
				Phase:          phase,
				VirtualMachine: vm,
			},
		}
	}

	validate := func(existing *v1alpha2.VirtualMachineMACAddress) error {
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
		v := NewVirtualMachineMACAddressOverrideValidator(newTemplate(), cl, "restore-uid")
		return v.ValidateWithForce(context.Background())
	}

	It("reuses an attached but orphaned MAC (VirtualMachine empty) instead of conflicting", func() {
		Expect(validate(existing(v1alpha2.VirtualMachineMACAddressPhaseAttached, ""))).
			To(MatchError(ErrAlreadyExists))
	})

	It("reuses a MAC already attached to the restored VM", func() {
		Expect(validate(existing(v1alpha2.VirtualMachineMACAddressPhaseAttached, restoreVM))).
			To(MatchError(ErrAlreadyExists))
	})

	It("conflicts when the MAC is bound to a different VM", func() {
		Expect(validate(existing(v1alpha2.VirtualMachineMACAddressPhaseAttached, "some-other-vm"))).
			To(MatchError(ErrAlreadyInUse))
	})

	It("recreates an unattached orphan MAC", func() {
		Expect(validate(existing("", ""))).To(Succeed())
	})
})
