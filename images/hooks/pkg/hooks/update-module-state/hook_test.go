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

package update_module_state

import (
	"context"
	"encoding/base64"
	"testing"

	"hooks/pkg/settings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUpdateModuleState(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Update Module State Suite")
}

var _ = Describe("Update Module State hook", func() {
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

	Context("when generic vmclass exists", func() {
		BeforeEach(func() {
			snapshots.GetMock.When(vmClassSnapshot).Then([]pkg.Snapshot{
				mock.NewSnapshotMock(GinkgoT()),
			})
		})

		It("should create module-state secret when it doesn't exist", func() {
			snapshots.GetMock.When(moduleStateSecretSnapshot).Then([]pkg.Snapshot{})

			patchCollector.CreateMock.Set(func(obj interface{}) {
				secret, ok := obj.(*corev1.Secret)
				Expect(ok).To(BeTrue())
				Expect(secret.Name).To(Equal("module-state"))
				Expect(secret.Namespace).To(Equal(settings.ModuleNamespace))
				Expect(secret.Data).To(HaveKey("generic-vmclass-was-ever-created"))

				Expect(string(secret.Data["generic-vmclass-was-ever-created"])).To(Equal("true"))
			})

			patchCollector.PatchWithMergeMock.Optional()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(1))
			Expect(patchCollector.PatchWithMergeMock.Calls()).To(HaveLen(0))
		})

		It("should update module-state secret when it exists but has wrong value", func() {
			moduleStateSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "module-state",
					Namespace: settings.ModuleNamespace,
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

			patchCollector.PatchWithMergeMock.Set(func(obj interface{}, apiVersion, kind, namespace, name string, opts ...pkg.PatchCollectorOption) {
				patchData, ok := obj.(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(patchData).To(HaveKey("data"))

				data, ok := patchData["data"].(map[string]string)
				Expect(ok).To(BeTrue())
				Expect(data).To(HaveKey("generic-vmclass-was-ever-created"))

				Expect(data["generic-vmclass-was-ever-created"]).To(Equal(base64.StdEncoding.EncodeToString([]byte("true"))))
			})

			patchCollector.CreateMock.Optional()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.PatchWithMergeMock.Calls()).To(HaveLen(1))
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
		})

		It("should update module-state secret even when it has correct value", func() {
			moduleStateSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "module-state",
					Namespace: settings.ModuleNamespace,
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

			patchCollector.PatchWithMergeMock.Set(func(obj interface{}, apiVersion, kind, namespace, name string, opts ...pkg.PatchCollectorOption) {
				patchData, ok := obj.(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(patchData).To(HaveKey("data"))

				data, ok := patchData["data"].(map[string]string)
				Expect(ok).To(BeTrue())
				Expect(data).To(HaveKey("generic-vmclass-was-ever-created"))

				Expect(data["generic-vmclass-was-ever-created"]).To(Equal(base64.StdEncoding.EncodeToString([]byte("true"))))
			})

			patchCollector.CreateMock.Optional()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.PatchWithMergeMock.Calls()).To(HaveLen(1))
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
		})
	})

	Context("when generic vmclass doesn't exist", func() {
		BeforeEach(func() {
			snapshots.GetMock.When(vmClassSnapshot).Then([]pkg.Snapshot{})
		})

		It("should create module-state secret even when vmclass doesn't exist and secret doesn't exist", func() {
			snapshots.GetMock.When(moduleStateSecretSnapshot).Then([]pkg.Snapshot{})

			patchCollector.CreateMock.Set(func(obj interface{}) {
				secret, ok := obj.(*corev1.Secret)
				Expect(ok).To(BeTrue())
				Expect(secret.Name).To(Equal("module-state"))
				Expect(secret.Namespace).To(Equal(settings.ModuleNamespace))
				Expect(secret.Data).To(HaveKey("generic-vmclass-was-ever-created"))

				Expect(string(secret.Data["generic-vmclass-was-ever-created"])).To(Equal("false"))
			})

			patchCollector.PatchWithMergeMock.Optional()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(1))
			Expect(patchCollector.PatchWithMergeMock.Calls()).To(HaveLen(0))
		})

		It("should update module-state secret and keep historical record when vmclass doesn't exist but module-state indicates it was created", func() {
			moduleStateSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "module-state",
					Namespace: settings.ModuleNamespace,
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

			patchCollector.PatchWithMergeMock.Set(func(obj interface{}, apiVersion, kind, namespace, name string, opts ...pkg.PatchCollectorOption) {
				patchData, ok := obj.(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(patchData).To(HaveKey("data"))

				data, ok := patchData["data"].(map[string]string)
				Expect(ok).To(BeTrue())
				Expect(data).To(HaveKey("generic-vmclass-created"))

				// Should keep historical record (true) even though VMClass doesn't exist now
				Expect(data["generic-vmclass-created"]).To(Equal(base64.StdEncoding.EncodeToString([]byte("true"))))
			})

			patchCollector.CreateMock.Optional()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.PatchWithMergeMock.Calls()).To(HaveLen(1))
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
		})

		It("should update module-state secret when vmclass doesn't exist and secret contains false", func() {
			moduleStateSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "module-state",
					Namespace: settings.ModuleNamespace,
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

			patchCollector.PatchWithMergeMock.Set(func(obj interface{}, apiVersion, kind, namespace, name string, opts ...pkg.PatchCollectorOption) {
				patchData, ok := obj.(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(patchData).To(HaveKey("data"))

				data, ok := patchData["data"].(map[string]string)
				Expect(ok).To(BeTrue())
				Expect(data).To(HaveKey("generic-vmclass-was-ever-created"))

				// Should remain false since VMClass doesn't exist
				Expect(data["generic-vmclass-was-ever-created"]).To(Equal(base64.StdEncoding.EncodeToString([]byte("false"))))
			})

			patchCollector.CreateMock.Optional()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.PatchWithMergeMock.Calls()).To(HaveLen(1))
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
		})
	})

	Context("state transition logic", func() {
		It("should preserve historical true value even when vmclass is deleted and recreated", func() {
			// First, simulate that VMClass was created and state recorded as true
			moduleStateSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "module-state",
					Namespace: settings.ModuleNamespace,
				},
				Data: map[string][]byte{
					"generic-vmclass-was-ever-created": []byte("true"),
				},
			}

			// VMClass doesn't exist now (was deleted)
			snapshots.GetMock.When(vmClassSnapshot).Then([]pkg.Snapshot{})

			snapshots.GetMock.When(moduleStateSecretSnapshot).Then([]pkg.Snapshot{
				mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) error {
					secret, ok := v.(*corev1.Secret)
					Expect(ok).To(BeTrue())
					*secret = *moduleStateSecret
					return nil
				}),
			})

			patchCollector.PatchWithMergeMock.Set(func(obj interface{}, apiVersion, kind, namespace, name string, opts ...pkg.PatchCollectorOption) {
				patchData, ok := obj.(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(patchData).To(HaveKey("data"))

				data, ok := patchData["data"].(map[string]string)
				Expect(ok).To(BeTrue())
				Expect(data).To(HaveKey("generic-vmclass-created"))

				// Should preserve historical true value even though VMClass doesn't exist
				Expect(data["generic-vmclass-created"]).To(Equal(base64.StdEncoding.EncodeToString([]byte("true"))))
			})

			patchCollector.CreateMock.Optional()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.PatchWithMergeMock.Calls()).To(HaveLen(1))
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
		})
	})
})
