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
				Expect(secret.Data).To(HaveKey("generic-vmclass-created"))

				decoded, err := base64.StdEncoding.DecodeString(string(secret.Data["generic-vmclass-created"]))
				Expect(err).ToNot(HaveOccurred())
				Expect(string(decoded)).To(Equal("true"))
			})

			patchCollector.PatchWithMergeMock.Optional()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(1))
			Expect(patchCollector.PatchWithMergeMock.Calls()).To(HaveLen(0))
		})

		It("should update module-state secret when it exists but has wrong value", func() {
			moduleStateData := map[string]interface{}{
				"generic-vmclass-created": base64.StdEncoding.EncodeToString([]byte("false")),
			}

			snapshots.GetMock.When(moduleStateSecretSnapshot).Then([]pkg.Snapshot{
				mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) error {
					data, ok := v.(*map[string]interface{})
					Expect(ok).To(BeTrue())
					*data = moduleStateData
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

				decoded, err := base64.StdEncoding.DecodeString(data["generic-vmclass-created"])
				Expect(err).ToNot(HaveOccurred())
				Expect(string(decoded)).To(Equal("true"))
			})

			patchCollector.CreateMock.Optional()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.PatchWithMergeMock.Calls()).To(HaveLen(1))
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
		})

		It("should not update module-state secret when it has correct value", func() {
			moduleStateData := map[string]interface{}{
				"generic-vmclass-created": base64.StdEncoding.EncodeToString([]byte("true")),
			}

			snapshots.GetMock.When(moduleStateSecretSnapshot).Then([]pkg.Snapshot{
				mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) error {
					data, ok := v.(*map[string]interface{})
					Expect(ok).To(BeTrue())
					*data = moduleStateData
					return nil
				}),
			})

			patchCollector.CreateMock.Optional()
			patchCollector.PatchWithMergeMock.Optional()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
			Expect(patchCollector.PatchWithMergeMock.Calls()).To(HaveLen(0))
		})
	})

	Context("when generic vmclass doesn't exist", func() {
		BeforeEach(func() {
			snapshots.GetMock.When(vmClassSnapshot).Then([]pkg.Snapshot{})
		})

		It("should create module-state secret with false value when it doesn't exist", func() {
			snapshots.GetMock.When(moduleStateSecretSnapshot).Then([]pkg.Snapshot{})

			patchCollector.CreateMock.Set(func(obj interface{}) {
				secret, ok := obj.(*corev1.Secret)
				Expect(ok).To(BeTrue())
				Expect(secret.Name).To(Equal("module-state"))
				Expect(secret.Namespace).To(Equal(settings.ModuleNamespace))
				Expect(secret.Data).To(HaveKey("generic-vmclass-created"))

				decoded, err := base64.StdEncoding.DecodeString(string(secret.Data["generic-vmclass-created"]))
				Expect(err).ToNot(HaveOccurred())
				Expect(string(decoded)).To(Equal("false"))
			})

			patchCollector.PatchWithMergeMock.Optional()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(1))
			Expect(patchCollector.PatchWithMergeMock.Calls()).To(HaveLen(0))
		})

		It("should update module-state secret when it exists but has wrong value", func() {
			moduleStateData := map[string]interface{}{
				"generic-vmclass-created": base64.StdEncoding.EncodeToString([]byte("true")),
			}

			snapshots.GetMock.When(moduleStateSecretSnapshot).Then([]pkg.Snapshot{
				mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) error {
					data, ok := v.(*map[string]interface{})
					Expect(ok).To(BeTrue())
					*data = moduleStateData
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

				decoded, err := base64.StdEncoding.DecodeString(data["generic-vmclass-created"])
				Expect(err).ToNot(HaveOccurred())
				Expect(string(decoded)).To(Equal("false"))
			})

			patchCollector.CreateMock.Optional()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.PatchWithMergeMock.Calls()).To(HaveLen(1))
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
		})
	})
})
