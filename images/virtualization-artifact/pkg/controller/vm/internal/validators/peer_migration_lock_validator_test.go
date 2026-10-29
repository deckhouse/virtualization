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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("PeerMigrationLockValidator", func() {
	newVM := func(ann map[string]string) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "app-x", Annotations: ann},
			Spec:       v1alpha2.VirtualMachineSpec{RunPolicy: v1alpha2.AlwaysOnPolicy},
		}
	}
	var v *PeerMigrationLockValidator
	BeforeEach(func() {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		v = NewPeerMigrationLockValidator(fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&v1alpha2.VirtualMachineOperation{
				ObjectMeta: metav1.ObjectMeta{Name: "move", Namespace: "app-x"},
				Spec:       v1alpha2.VirtualMachineOperationSpec{Type: v1alpha2.VMOPTypeMigrate, VirtualMachine: "web", Migrate: &v1alpha2.VirtualMachineOperationMigrateSpec{ProjectPeerName: "cluster-b"}},
			}).Build())
	})

	It("refuses a change of a VM that moves to a peer cluster, naming the cluster", func() {
		oldVM := newVM(map[string]string{annotations.AnnVMMovedToPeer: "move"})
		newVM := oldVM.DeepCopy()
		newVM.Spec.RunPolicy = v1alpha2.ManualPolicy
		_, err := v.ValidateUpdate(context.Background(), oldVM, newVM)
		Expect(err).To(MatchError(ContainSubstring(`migrated to cluster "cluster-b"`)))
	})

	as := func(user string) context.Context {
		return admission.NewContextWithRequest(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: authnv1.UserInfo{Username: user}}})
	}

	It("lets the move release the lock together with a change", func() {
		oldVM := newVM(map[string]string{annotations.AnnVMMovedToPeer: "move"})
		newVM := newVM(nil)
		newVM.Spec.RunPolicy = v1alpha2.ManualPolicy
		_, err := v.ValidateUpdate(as(controllerUser), oldVM, newVM)
		Expect(err).NotTo(HaveOccurred())
	})

	It("refuses a user to release the lock, even without a change of the spec", func() {
		oldVM := newVM(map[string]string{annotations.AnnVMMovedToPeer: "move"})
		_, err := v.ValidateUpdate(as("editor"), oldVM, newVM(nil))
		Expect(err).To(MatchError(ContainSubstring(annotations.AnnVMMovedToPeer)))
	})

	It("refuses a user to release a VM that is being received", func() {
		oldVM := newVM(map[string]string{annotations.AnnVMMigrationReceiver: "uid-1"})
		_, err := v.ValidateUpdate(as("editor"), oldVM, newVM(nil))
		Expect(err).To(MatchError(ContainSubstring(annotations.AnnVMMigrationReceiver)))
	})

	It("refuses a change of a VM that is being received from a peer cluster", func() {
		oldVM := newVM(map[string]string{annotations.AnnVMMigrationReceiver: "uid-1"})
		newVM := oldVM.DeepCopy()
		newVM.Spec.RunPolicy = v1alpha2.ManualPolicy
		_, err := v.ValidateUpdate(context.Background(), oldVM, newVM)
		Expect(err).To(MatchError(ContainSubstring("migrated from a peer cluster")))
	})

	It("ignores changes outside the spec", func() {
		oldVM := newVM(map[string]string{annotations.AnnVMMovedToPeer: "move"})
		newVM := oldVM.DeepCopy()
		newVM.Labels = map[string]string{"x": "y"}
		_, err := v.ValidateUpdate(context.Background(), oldVM, newVM)
		Expect(err).NotTo(HaveOccurred())
	})
})
