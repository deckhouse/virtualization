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

package validators

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	authnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("PeerMigrationLockValidator", func() {
	newVM := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app-x"},
			Spec:       v1alpha2.VirtualMachineSpec{RunPolicy: v1alpha2.AlwaysOnPolicy},
		}
	}

	var v *PeerMigrationLockValidator

	BeforeEach(func() {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(virtv1.AddToScheme(scheme)).To(Succeed())
		v = NewPeerMigrationLockValidator(fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&v1alpha2.VirtualMachineOperation{
				ObjectMeta: metav1.ObjectMeta{Name: "move", Namespace: "app-x"},
				Spec:       v1alpha2.VirtualMachineOperationSpec{Type: v1alpha2.VMOPTypeMigrate, VirtualMachine: "web", Migrate: &v1alpha2.VirtualMachineOperationMigrateSpec{ProjectPeerName: "cluster-b"}},
			},
			&virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app-x", Annotations: map[string]string{annotations.AnnPeerMigrationVMOP: "move"}}},
		).Build())
	})

	as := func(user string) context.Context {
		return admission.NewContextWithRequest(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: authnv1.UserInfo{Username: user}}})
	}

	It("refuses a change of a VM that moves to a peer cluster, naming the cluster", func() {
		oldVM := newVM()
		newVM := oldVM.DeepCopy()
		newVM.Spec.RunPolicy = v1alpha2.ManualPolicy
		_, err := v.ValidateUpdate(as("editor"), oldVM, newVM)
		Expect(err).To(MatchError(ContainSubstring(`migrated to peer cluster "cluster-b"`)))
	})

	// The move hands the VM back with the Manual run policy before it lifts the mark.
	It("lets the controller change a VM that moves", func() {
		oldVM := newVM()
		newVM := oldVM.DeepCopy()
		newVM.Spec.RunPolicy = v1alpha2.ManualPolicy
		_, err := v.ValidateUpdate(as(controllerUser), oldVM, newVM)
		Expect(err).NotTo(HaveOccurred())
	})

	It("refuses a change of a VM that is being received from a peer cluster", func() {
		oldVM := newVM()
		oldVM.Name = "db"
		oldVM.Status.Conditions = []metav1.Condition{{Type: vmcondition.TypeReceivingFromPeer.String(), Status: metav1.ConditionTrue}}
		newVM := oldVM.DeepCopy()
		newVM.Spec.RunPolicy = v1alpha2.ManualPolicy
		_, err := v.ValidateUpdate(as("editor"), oldVM, newVM)
		Expect(err).To(MatchError(ContainSubstring("migrated from a peer cluster")))
	})

	It("ignores changes outside the spec", func() {
		oldVM := newVM()
		newVM := oldVM.DeepCopy()
		newVM.Labels = map[string]string{"x": "y"}
		_, err := v.ValidateUpdate(as("editor"), oldVM, newVM)
		Expect(err).NotTo(HaveOccurred())
	})
})
