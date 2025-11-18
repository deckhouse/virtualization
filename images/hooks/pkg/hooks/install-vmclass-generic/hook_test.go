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

package install_vmclass_generic

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha3"
)

func Test_InstallVMClassGeneric(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Create Generic VMClass Suite")
}

var _ = Describe("Install VMClass Generic hook", func() {
	var (
		snapshots      *mock.SnapshotsMock
		values         *mock.OutputPatchableValuesCollectorMock
		patchCollector *mock.PatchCollectorMock
	)

	newInput := func() *pkg.HookInput {
		return &pkg.HookInput{
			Snapshots:      snapshots,
			Values:         values,
			PatchCollector: patchCollector,
			Logger:         log.NewNop(),
		}
	}

	prepareStateValuesEmpty := func() {
		values.GetMock.When(vmClassInstallationStateValuesPath).Then(gjson.Result{Type: gjson.Null})
	}

	prepareStateValuesInstalled := func() {
		values.GetMock.When(vmClassInstallationStateValuesPath).Then(gjson.Result{
			Type: gjson.String,
			Str:  `{"installedAt":"2020-01-01T00:00:00Z"}`,
		})
	}

	prepareModuleStateSnapshotEmpty := func() {
		snapshots.GetMock.When(moduleStateSecretSnapshot).Then([]pkg.Snapshot{})
	}

	prepareModuleStateSnapshotValid := func() {
		moduleStateSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "module-state",
				Namespace: "d8-virtualization",
			},
			Data: map[string][]byte{
				vmClassInstallationStateSecretKey: []byte(`{"installedAt":"2020-01-01T00:00:00Z"}`),
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
	}

	prepareModuleStateSnapshotNoVMClassState := func() {
		moduleStateSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "module-state",
				Namespace: "d8-virtualization",
			},
			Data: map[string][]byte{
				"other-key": []byte(`"other-value"`),
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
	}

	prepareVMClassSnapshotEmpty := func() {
		snapshots.GetMock.When(vmClassGenericSnapshot).Then([]pkg.Snapshot{})
	}

	prepareVMClassSnapshotGeneric := func() {
		vmClass := vmClassGenericManifest().DeepCopy()
		vmClass.Annotations = map[string]string{
			helmKeepResourceAnno: "keep",
		}
		snapshots.GetMock.When(vmClassGenericSnapshot).Then([]pkg.Snapshot{
			mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) error {
				vmClassInSnapshot, ok := v.(*v1alpha2.VirtualMachineClass)
				Expect(ok).To(BeTrue())
				*vmClassInSnapshot = *vmClass
				return nil
			}),
		})
	}

	prepareVMClassSnapshotGenericWithoutKeepResource := func() {
		vmClass := vmClassGenericManifest().DeepCopy()
		snapshots.GetMock.When(vmClassGenericSnapshot).Then([]pkg.Snapshot{
			mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) error {
				vmClassInSnapshot, ok := v.(*v1alpha2.VirtualMachineClass)
				Expect(ok).To(BeTrue())
				*vmClassInSnapshot = *vmClass
				return nil
			}),
		})
	}

	prepareVMClassSnapshotCustom := func() {
		vmClass := vmClassGenericManifest().DeepCopy()
		vmClass.Labels = map[string]string{
			"created-by": "user",
		}
		vmClass.Annotations = nil
		snapshots.GetMock.When(vmClassGenericSnapshot).Then([]pkg.Snapshot{
			mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) error {
				vmClassInSnapshot, ok := v.(*v1alpha2.VirtualMachineClass)
				Expect(ok).To(BeTrue())
				*vmClassInSnapshot = *vmClass
				return nil
			}),
		})
	}

	prepareVMClassSnapshotGenericHelmManaged := func() {
		vmClass := vmClassGenericManifest().DeepCopy()
		// Keep app, heritage, and module labels.
		vmClass.Labels[helmManagedByLabel] = "Helm"
		vmClass.Annotations = map[string]string{
			helmReleaseNameAnno:      "somename",
			helmReleaseNamespaceAnno: "some ns",
		}
		snapshots.GetMock.When(vmClassGenericSnapshot).Then([]pkg.Snapshot{
			mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) error {
				vmClassInSnapshot, ok := v.(*v1alpha2.VirtualMachineClass)
				Expect(ok).To(BeTrue())
				*vmClassInSnapshot = *vmClass
				return nil
			}),
		})
	}

	prepareVMClassSnapshotGenericCustomHelmManaged := func() {
		vmClass := vmClassGenericManifest().DeepCopy()
		vmClass.Labels = map[string]string{
			"created-by":       "user",
			helmManagedByLabel: "Helm",
		}
		vmClass.Annotations = map[string]string{
			helmReleaseNameAnno:      "somename",
			helmReleaseNamespaceAnno: "some ns",
		}
		snapshots.GetMock.When(vmClassGenericSnapshot).Then([]pkg.Snapshot{
			mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) error {
				vmClassInSnapshot, ok := v.(*v1alpha2.VirtualMachineClass)
				Expect(ok).To(BeTrue())
				*vmClassInSnapshot = *vmClass
				return nil
			}),
		})
	}

	expectVMClassGenericV3 := func(obj interface{}) {
		GinkgoHelper()
		vmClass, ok := obj.(*v1alpha3.VirtualMachineClass)
		Expect(ok).To(BeTrue())
		Expect(vmClass.Name).To(Equal("generic"))
		Expect(vmClass.Labels).To(Equal(map[string]string{
			"app":    "virtualization-controller",
			"module": "virtualization",
		}))
		Expect(vmClass.Spec.SizingPolicies).To(HaveLen(4))
		Expect(vmClass.Spec.SizingPolicies[0].CoreFractions).To(Equal([]v1alpha3.CoreFractionValue{"5%", "10%", "20%", "50%", "100%"}))
	}

	BeforeEach(func() {
		snapshots = mock.NewSnapshotsMock(GinkgoT())
		values = mock.NewPatchableValuesCollectorMock(GinkgoT())
		patchCollector = mock.NewPatchCollectorMock(GinkgoT())
	})

	AfterEach(func() {
		snapshots = nil
		values = nil
		patchCollector = nil
	})

	When("module-state secret has the vmclass state", func() {
		It("should set values and not recreate or patch vmclass/generic", func() {
			prepareModuleStateSnapshotValid()

			patchCollector.CreateMock.Optional()
			patchCollector.PatchWithJSONMock.Optional()
			values.SetMock.Return()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
			Expect(patchCollector.PatchWithJSONMock.Calls()).To(HaveLen(0))
			Expect(values.SetMock.Calls()).To(HaveLen(1), "should set values from the Secret")
		})
	})

	When("no module-state secret and no vmclass", func() {
		BeforeEach(func() {
			prepareModuleStateSnapshotEmpty()
		})

		When("no state in values and no vmclass", func() {
			It("should create vmclass/generic v1alpha3 and set values", func() {
				prepareVMClassSnapshotEmpty()
				prepareStateValuesEmpty()

				values.SetMock.Return()
				patchCollector.CreateMock.Set(expectVMClassGenericV3)

				Expect(Reconcile(context.Background(), newInput())).To(Succeed())
				Expect(patchCollector.CreateMock.Calls()).To(HaveLen(1), "should call Create once")
				Expect(values.SetMock.Calls()).To(HaveLen(1), "should call values.Set once")
			})
		})
		When("state is present in values", func() {
			It("should not create vmclass/generic ans set values", func() {
				prepareStateValuesInstalled()

				values.SetMock.Optional()
				patchCollector.CreateMock.Optional()

				Expect(Reconcile(context.Background(), newInput())).To(Succeed())
				Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
				Expect(values.SetMock.Calls()).To(HaveLen(0))
			})
		})
	})

	When("module-state secret is present without vmclass state", func() {
		BeforeEach(func() {
			prepareModuleStateSnapshotNoVMClassState()
		})

		When("state is in values", func() {
			It("should not change vmclass/generic", func() {
				prepareStateValuesInstalled()

				values.SetMock.Optional()
				patchCollector.CreateMock.Optional()
				patchCollector.PatchWithJSONMock.Optional()

				Expect(Reconcile(context.Background(), newInput())).To(Succeed())
				Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
				Expect(patchCollector.PatchWithJSONMock.Calls()).To(HaveLen(0))
				Expect(values.SetMock.Calls()).To(HaveLen(0))
			})
		})

		When("no state in values", func() {
			BeforeEach(func() {
				prepareStateValuesEmpty()
			})

			When("no vmclass/generic", func() {
				It("should create vmclass/generic v1alpha3 and set values", func() {
					prepareVMClassSnapshotEmpty()

					values.SetMock.Return()
					patchCollector.CreateMock.Set(expectVMClassGenericV3)

					Expect(Reconcile(context.Background(), newInput())).To(Succeed())
					Expect(patchCollector.CreateMock.Calls()).To(HaveLen(1))
					Expect(values.SetMock.Calls()).To(HaveLen(1))
				})
			})

			When("vmclass/generic is present", func() {
				It("should not change vmclass/generic and set values", func() {
					prepareVMClassSnapshotGeneric()

					values.SetMock.Return()
					patchCollector.CreateMock.Optional()
					patchCollector.PatchWithJSONMock.Optional()

					Expect(Reconcile(context.Background(), newInput())).To(Succeed())
					Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
					Expect(patchCollector.PatchWithJSONMock.Calls()).To(HaveLen(0))
					Expect(values.SetMock.Calls()).To(HaveLen(1))
				})
			})

			When("vmclass/generic without keep-resource annotation is present", func() {
				It("should not change vmclass/generic and set values", func() {
					prepareVMClassSnapshotGenericWithoutKeepResource()

					values.SetMock.Return()
					patchCollector.CreateMock.Optional()
					patchCollector.PatchWithJSONMock.Return()

					Expect(Reconcile(context.Background(), newInput())).To(Succeed())
					Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
					Expect(patchCollector.PatchWithJSONMock.Calls()).To(HaveLen(1))
					Expect(values.SetMock.Calls()).To(HaveLen(1))
				})
			})

			When("vmclass/generic has helm label", func() {
				It("should set values and remove helm labels", func() {
					prepareVMClassSnapshotGenericHelmManaged()

					patchCollector.CreateMock.Optional()
					patchCollector.PatchWithJSONMock.Return()
					values.SetMock.Return()

					Expect(Reconcile(context.Background(), newInput())).To(Succeed())
					Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
					Expect(patchCollector.PatchWithJSONMock.Calls()).To(HaveLen(1))
					Expect(values.SetMock.Calls()).To(HaveLen(1), "should set values from the Secret")
				})
			})

			When("custom vmclass/generic is present", func() {
				It("should set values and not patch vmclass/generic", func() {
					prepareVMClassSnapshotCustom()

					patchCollector.CreateMock.Optional()
					patchCollector.PatchWithJSONMock.Optional()
					values.SetMock.Return()

					Expect(Reconcile(context.Background(), newInput())).To(Succeed())
					Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
					Expect(patchCollector.PatchWithJSONMock.Calls()).To(HaveLen(0))
					Expect(values.SetMock.Calls()).To(HaveLen(1), "should set values from the Secret")
				})
			})

			When("custom vmclass/generic has helm label", func() {
				It("should set values and not remove helm values", func() {
					prepareVMClassSnapshotGenericCustomHelmManaged()

					patchCollector.CreateMock.Optional()
					patchCollector.PatchWithJSONMock.Optional()
					values.SetMock.Return()

					Expect(Reconcile(context.Background(), newInput())).To(Succeed())
					Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
					Expect(patchCollector.PatchWithJSONMock.Calls()).To(HaveLen(0))
					Expect(values.SetMock.Calls()).To(HaveLen(1), "should set values from the Secret")
				})
			})
		})
	})
})
