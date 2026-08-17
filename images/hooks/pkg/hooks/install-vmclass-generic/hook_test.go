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
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
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
		dc             *mock.DependencyContainerMock
	)

	newInput := func() *pkg.HookInput {
		return &pkg.HookInput{
			Snapshots:      snapshots,
			Values:         values,
			PatchCollector: patchCollector,
			DC:             dc,
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

	prepareVMClassInCluster := func(vmClass *v1alpha2.VirtualMachineClass) {
		dc.GetK8sClientMock.Return(&fakeKubernetesClient{get: func(_ context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object) error {
			Expect(key.Name).To(Equal(vmClassGenericName))
			vmClassToGet, ok := obj.(*v1alpha2.VirtualMachineClass)
			Expect(ok).To(BeTrue())
			*vmClassToGet = *vmClass
			return nil
		}}, nil)
	}

	prepareVMClassAbsent := func() {
		dc.GetK8sClientMock.Return(&fakeKubernetesClient{get: func(_ context.Context, _ ctrlclient.ObjectKey, _ ctrlclient.Object) error {
			return apierrors.NewNotFound(
				schema.GroupResource{Group: v1alpha2.SchemeGroupVersion.Group, Resource: "virtualmachineclasses"},
				vmClassGenericName,
			)
		}}, nil)
	}

	// prepareVMClassReadFailure mimics a failed conversion webhook: the API server cannot
	// serve VirtualMachineClass while the Service of virtualization-controller is absent.
	prepareVMClassReadFailure := func() {
		dc.GetK8sClientMock.Return(&fakeKubernetesClient{get: func(_ context.Context, _ ctrlclient.ObjectKey, _ ctrlclient.Object) error {
			return errors.New(`conversion webhook for virtualization.deckhouse.io/v1alpha2, Kind=VirtualMachineClass failed: service "virtualization-controller" not found`)
		}}, nil)
	}

	prepareVMClassGeneric := func() {
		vmClass := vmClassGenericManifest().DeepCopy()
		vmClass.Annotations = map[string]string{
			helmKeepResourceAnno: "keep",
		}
		prepareVMClassInCluster(vmClass)
	}

	prepareVMClassGenericWithoutKeepResource := func() {
		prepareVMClassInCluster(vmClassGenericManifest().DeepCopy())
	}

	prepareVMClassCustom := func() {
		vmClass := vmClassGenericManifest().DeepCopy()
		vmClass.Labels = map[string]string{
			"created-by": "user",
		}
		vmClass.Annotations = nil
		prepareVMClassInCluster(vmClass)
	}

	prepareVMClassGenericHelmManaged := func() {
		vmClass := vmClassGenericManifest().DeepCopy()
		// Keep app, heritage, and module labels.
		vmClass.Labels[helmManagedByLabel] = "Helm"
		vmClass.Annotations = map[string]string{
			helmReleaseNameAnno:      "somename",
			helmReleaseNamespaceAnno: "some ns",
		}
		prepareVMClassInCluster(vmClass)
	}

	prepareVMClassGenericCustomHelmManaged := func() {
		vmClass := vmClassGenericManifest().DeepCopy()
		vmClass.Labels = map[string]string{
			"created-by":       "user",
			helmManagedByLabel: "Helm",
		}
		vmClass.Annotations = map[string]string{
			helmReleaseNameAnno:      "somename",
			helmReleaseNamespaceAnno: "some ns",
		}
		prepareVMClassInCluster(vmClass)
	}

	expectVMClassGeneric := func(obj interface{}) {
		GinkgoHelper()
		vmClass, ok := obj.(*v1alpha2.VirtualMachineClass)
		Expect(ok).To(BeTrue())
		Expect(vmClass.Name).To(Equal("generic"))
		Expect(vmClass.Labels).To(Equal(map[string]string{
			"app":    "virtualization-controller",
			"module": "virtualization",
		}))
	}

	BeforeEach(func() {
		snapshots = mock.NewSnapshotsMock(GinkgoT())
		values = mock.NewOutputPatchableValuesCollectorMock(GinkgoT())
		patchCollector = mock.NewPatchCollectorMock(GinkgoT())
		dc = mock.NewDependencyContainerMock(GinkgoT())
	})

	AfterEach(func() {
		snapshots = nil
		values = nil
		patchCollector = nil
		dc = nil
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
			It("should create vmclass/generic and set values", func() {
				prepareVMClassAbsent()
				prepareStateValuesEmpty()

				values.SetMock.Return()
				patchCollector.CreateMock.Set(expectVMClassGeneric)

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
				It("should create vmclass/generic and set values", func() {
					prepareVMClassAbsent()

					values.SetMock.Return()
					patchCollector.CreateMock.Set(expectVMClassGeneric)

					Expect(Reconcile(context.Background(), newInput())).To(Succeed())
					Expect(patchCollector.CreateMock.Calls()).To(HaveLen(1))
					Expect(values.SetMock.Calls()).To(HaveLen(1))
				})
			})

			When("vmclass/generic cannot be read", func() {
				It("should succeed without touching vmclass/generic and values", func() {
					prepareVMClassReadFailure()

					values.SetMock.Optional()
					patchCollector.CreateMock.Optional()
					patchCollector.PatchWithJSONMock.Optional()

					Expect(Reconcile(context.Background(), newInput())).To(Succeed(), "should not stop the module startup")
					Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0), "a read failure is not an evidence of absence")
					Expect(patchCollector.PatchWithJSONMock.Calls()).To(HaveLen(0))
					Expect(values.SetMock.Calls()).To(HaveLen(0), "state should stay unset to retry on the next run")
				})
			})

			When("vmclass/generic is present", func() {
				It("should not change vmclass/generic and set values", func() {
					prepareVMClassGeneric()

					values.SetMock.Return()
					patchCollector.CreateMock.Optional()
					patchCollector.PatchWithJSONMock.Optional()

					Expect(Reconcile(context.Background(), newInput())).To(Succeed())
					Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
					Expect(patchCollector.PatchWithJSONMock.Calls()).To(HaveLen(0))
					Expect(values.SetMock.Calls()).To(HaveLen(1))
				})
			})

			When("vmclass/generic without keep-resource annotation", func() {
				It("should not change vmclass/generic and set values", func() {
					prepareVMClassGenericWithoutKeepResource()

					values.SetMock.Return()
					patchCollector.CreateMock.Optional()
					patchCollector.PatchWithJSONMock.Optional()

					Expect(Reconcile(context.Background(), newInput())).To(Succeed())
					Expect(patchCollector.CreateMock.Calls()).To(HaveLen(0))
					Expect(patchCollector.PatchWithJSONMock.Calls()).To(HaveLen(0))
					Expect(values.SetMock.Calls()).To(HaveLen(1))
				})
			})

			When("vmclass/generic has helm label", func() {
				It("should set values and remove helm labels", func() {
					prepareVMClassGenericHelmManaged()

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
					prepareVMClassCustom()

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
					prepareVMClassGenericCustomHelmManaged()

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

type fakeKubernetesClient struct {
	pkg.KubernetesClient
	get func(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object) error
}

func (f *fakeKubernetesClient) Get(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object, _ ...ctrlclient.GetOption) error {
	return f.get(ctx, key, obj)
}
