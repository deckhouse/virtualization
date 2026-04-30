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

package validator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/config"
	basevc "github.com/deckhouse/virtualization-controller/pkg/controller/service"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("VirtualImagePVCStorageClassValidator", func() {
	It("should use the default storage class when VirtualDisk storage class is not set", func() {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())

		defaultSC := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default-sc",
				Annotations: map[string]string{
					annotations.AnnDefaultStorageClass: "true",
				},
			},
		}
		vi := &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-vi",
				Namespace: "default",
			},
			Spec: v1alpha2.VirtualImageSpec{
				Storage: v1alpha2.StoragePersistentVolumeClaim,
			},
			Status: v1alpha2.VirtualImageStatus{
				Phase:            v1alpha2.ImageReady,
				StorageClassName: defaultSC.Name,
			},
		}

		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "target-vd",
				Namespace: "default",
			},
			Spec: v1alpha2.VirtualDiskSpec{
				DataSource: &v1alpha2.VirtualDiskDataSource{
					Type: v1alpha2.DataSourceTypeObjectRef,
					ObjectRef: &v1alpha2.VirtualDiskObjectRef{
						Kind: v1alpha2.VirtualDiskObjectRefKindVirtualImage,
						Name: vi.Name,
					},
				},
			},
		}

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(defaultSC, vi).Build()
		baseSCService := basevc.NewBaseStorageClassService(k8sClient)
		vdSCService := intsvc.NewVirtualDiskStorageClassService(baseSCService, config.VirtualDiskStorageClassSettings{})
		validator := NewVirtualImagePVCStorageClassValidator(k8sClient, vdSCService)

		var err error
		Expect(func() {
			_, err = validator.ValidateCreate(context.Background(), vd)
		}).NotTo(Panic())
		Expect(err).NotTo(HaveOccurred())
	})

	It("should return a readable mismatch error", func() {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())

		vi := &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-vi",
				Namespace: "default",
			},
			Spec: v1alpha2.VirtualImageSpec{
				Storage: v1alpha2.StoragePersistentVolumeClaim,
			},
			Status: v1alpha2.VirtualImageStatus{
				Phase:            v1alpha2.ImageReady,
				StorageClassName: "vi-sc",
			},
		}
		vdSC := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vd-sc",
			},
			Provisioner: "first.csi.example.com",
		}
		viSC := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vi-sc",
			},
			Provisioner: "second.csi.example.com",
		}
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "target-vd",
				Namespace: "default",
			},
			Spec: v1alpha2.VirtualDiskSpec{
				PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{
					StorageClass: func() *string {
						sc := "vd-sc"
						return &sc
					}(),
				},
				DataSource: &v1alpha2.VirtualDiskDataSource{
					Type: v1alpha2.DataSourceTypeObjectRef,
					ObjectRef: &v1alpha2.VirtualDiskObjectRef{
						Kind: v1alpha2.VirtualDiskObjectRefKindVirtualImage,
						Name: vi.Name,
					},
				},
			},
		}

		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vi, vdSC, viSC).Build()
		baseSCService := basevc.NewBaseStorageClassService(k8sClient)
		vdSCService := intsvc.NewVirtualDiskStorageClassService(baseSCService, config.VirtualDiskStorageClassSettings{})
		validator := NewVirtualImagePVCStorageClassValidator(k8sClient, vdSCService)

		_, err := validator.ValidateCreate(context.Background(), vd)
		Expect(err).To(MatchError(`virtual disk storage class "vd-sc" provisioner does not match virtual image storage class "vi-sc" provisioner: source type with different provisioners is not supported yet`))
	})
})
