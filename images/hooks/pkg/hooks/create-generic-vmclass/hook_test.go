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

package create_generic_vmclass

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateGenericVMClass(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Create Generic VMClass Suite")
}

var _ = Describe("Create Generic VMClass hook", func() {
	var (
		snapshots      *mock.SnapshotsMock
		patchCollector *mock.PatchCollectorMock
	)

	newInput := func() *pkg.HookInput {
		return &pkg.HookInput{
			Snapshots:      snapshots,
			PatchCollector: patchCollector,
			Logger:         log.NewNop(),
		}
	}

	BeforeEach(func() {
		snapshots = mock.NewSnapshotsMock(GinkgoT())
		patchCollector = mock.NewPatchCollectorMock(GinkgoT())
	})

	AfterEach(func() {
		snapshots = nil
		patchCollector = nil
	})

	Context("when module-state secret exists with generic-vmclass-was-ever-created=true", func() {
		BeforeEach(func() {
			moduleStateSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "module-state",
					Namespace: "d8-virtualization",
				},
				Data: map[string][]byte{
					"generic-vmclass-was-ever-created": []byte("true"),
				},
			}

			snapshots.GetMock.When(moduleStateSecretSnapshot).Then([]pkg.Snapshot{
				mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) error {
					secret, ok := v.(*corev1.Secret)
					Expect(ok).To(BeTrue())
					*secret = *moduleStateSecret
					return nil
				}),
			})
		})

		It("should not recreate generic vmclass when it doesn't exist but state says it was created (user may have deleted it intentionally)", func() {
			snapshots.GetMock.When(vmClassSnapshot).Then([]pkg.Snapshot{})

			patchCollector.CreateMock.Optional()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
		})

	})

	Context("when module-state secret doesn't exist", func() {
		BeforeEach(func() {
			snapshots.GetMock.When(moduleStateSecretSnapshot).Then([]pkg.Snapshot{})
		})

		It("should create generic vmclass when it doesn't exist", func() {
			snapshots.GetMock.When(vmClassSnapshot).Then([]pkg.Snapshot{})

			patchCollector.CreateMock.Set(func(obj interface{}) {
				vmClass, ok := obj.(*v1alpha2.VirtualMachineClass)
				Expect(ok).To(BeTrue())
				Expect(vmClass.Name).To(Equal("generic"))
				Expect(vmClass.Labels).To(Equal(map[string]string{
					"app":    "virtualization-controller",
					"module": "virtualization",
				}))
			})

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(1))
		})

	})

	Context("when module-state secret exists but doesn't contain generic-vmclass-was-ever-created", func() {
		BeforeEach(func() {
			moduleStateSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "module-state",
					Namespace: "d8-virtualization",
				},
				Data: map[string][]byte{
					"other-key": []byte("other-value"),
				},
			}

			snapshots.GetMock.When(moduleStateSecretSnapshot).Then([]pkg.Snapshot{
				mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) error {
					secret, ok := v.(*corev1.Secret)
					Expect(ok).To(BeTrue())
					*secret = *moduleStateSecret
					return nil
				}),
			})
		})

		It("should create generic vmclass when it doesn't exist", func() {
			snapshots.GetMock.When(vmClassSnapshot).Then([]pkg.Snapshot{})

			patchCollector.CreateMock.Set(func(obj interface{}) {
				vmClass, ok := obj.(*v1alpha2.VirtualMachineClass)
				Expect(ok).To(BeTrue())
				Expect(vmClass.Name).To(Equal("generic"))
				Expect(vmClass.Labels).To(Equal(map[string]string{
					"app":    "virtualization-controller",
					"module": "virtualization",
				}))
			})

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(1))
		})

	})

	Context("when module-state secret exists with generic-vmclass-was-ever-created=false", func() {
		BeforeEach(func() {
			moduleStateSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "module-state",
					Namespace: "d8-virtualization",
				},
				Data: map[string][]byte{
					"generic-vmclass-was-ever-created": []byte("false"),
				},
			}

			snapshots.GetMock.When(moduleStateSecretSnapshot).Then([]pkg.Snapshot{
				mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) error {
					secret, ok := v.(*corev1.Secret)
					Expect(ok).To(BeTrue())
					*secret = *moduleStateSecret
					return nil
				}),
			})
		})

		It("should create generic vmclass when it doesn't exist", func() {
			snapshots.GetMock.When(vmClassSnapshot).Then([]pkg.Snapshot{})

			patchCollector.CreateMock.Set(func(obj interface{}) {
				vmClass, ok := obj.(*v1alpha2.VirtualMachineClass)
				Expect(ok).To(BeTrue())
				Expect(vmClass.Name).To(Equal("generic"))
				Expect(vmClass.Labels).To(Equal(map[string]string{
					"app":    "virtualization-controller",
					"module": "virtualization",
				}))
			})

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(1))
		})

	})
})
