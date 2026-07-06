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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/config"
	basevc "github.com/deckhouse/virtualization-controller/pkg/controller/service"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("VirtualImagePVCStorageClassValidator", func() {
	const (
		namespace = "default"
		viName    = "source-vi"
		vdName    = "target-vd"
	)

	newScheme := func() *runtime.Scheme {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())
		return scheme
	}

	ptr := func(v string) *string { return &v }

	newStorageClass := func(name, provisioner string, isDefault bool) *storagev1.StorageClass {
		sc := &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Provisioner: provisioner,
		}
		if isDefault {
			sc.Annotations = map[string]string{
				annotations.AnnDefaultStorageClass: "true",
			}
		}
		return sc
	}

	newVirtualImage := func() *v1alpha2.VirtualImage {
		return &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      viName,
				Namespace: namespace,
			},
			Spec: v1alpha2.VirtualImageSpec{
				Storage: v1alpha2.StoragePersistentVolumeClaim,
			},
			Status: v1alpha2.VirtualImageStatus{
				Phase:            v1alpha2.ImageReady,
				StorageClassName: "vi-sc",
			},
		}
	}

	newDataSource := func() *v1alpha2.VirtualDiskDataSource {
		return &v1alpha2.VirtualDiskDataSource{
			Type: v1alpha2.DataSourceTypeObjectRef,
			ObjectRef: &v1alpha2.VirtualDiskObjectRef{
				Kind: v1alpha2.VirtualDiskObjectRefKindVirtualImage,
				Name: viName,
			},
		}
	}

	newVD := func(statusSC string, specSC *string, ds *v1alpha2.VirtualDiskDataSource) *v1alpha2.VirtualDisk {
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vdName,
				Namespace: namespace,
			},
			Spec: v1alpha2.VirtualDiskSpec{
				PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{StorageClass: specSC},
				DataSource:            ds,
			},
		}
		vd.Status.StorageClassName = statusSC
		return vd
	}

	newValidator := func(settings config.VirtualDiskStorageClassSettings, objs ...client.Object) *VirtualImagePVCStorageClassValidator {
		k8sClient := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(objs...).Build()
		baseSCService := basevc.NewBaseStorageClassService(k8sClient)
		vdSCService := intsvc.NewVirtualDiskStorageClassService(baseSCService, settings)
		return NewVirtualImagePVCStorageClassValidator(k8sClient, vdSCService)
	}

	type updateCase struct {
		oldVD *v1alpha2.VirtualDisk
		newVD *v1alpha2.VirtualDisk
	}

	DescribeTable("ValidateCreate", func(vd *v1alpha2.VirtualDisk, settings config.VirtualDiskStorageClassSettings, objs []client.Object, expectedErr string) {
		validator := newValidator(settings, objs...)
		_, err := validator.ValidateCreate(context.Background(), vd)

		if expectedErr == "" {
			Expect(err).NotTo(HaveOccurred())
			return
		}

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(expectedErr))
	},
		Entry("uses status storage class first", func() *v1alpha2.VirtualDisk {
			return newVD("status-sc", ptr("spec-sc"), newDataSource())
		}(), config.VirtualDiskStorageClassSettings{}, []client.Object{
			newVirtualImage(),
			newStorageClass("status-sc", "csi.example.com", false),
			newStorageClass("spec-sc", "other.csi.example.com", false),
			newStorageClass("vi-sc", "csi.example.com", false),
		}, ""),
		Entry("uses spec storage class when status is empty", func() *v1alpha2.VirtualDisk {
			return newVD("", ptr("spec-sc"), newDataSource())
		}(), config.VirtualDiskStorageClassSettings{}, []client.Object{
			newVirtualImage(),
			newStorageClass("spec-sc", "csi.example.com", false),
			newStorageClass("default-sc", "other.csi.example.com", true),
			newStorageClass("vi-sc", "csi.example.com", false),
		}, ""),
		Entry("uses module storage class before default class", func() *v1alpha2.VirtualDisk {
			return newVD("", nil, newDataSource())
		}(), config.VirtualDiskStorageClassSettings{
			DefaultStorageClassName: "module-sc",
		}, []client.Object{
			newVirtualImage(),
			newStorageClass("module-sc", "csi.example.com", false),
			newStorageClass("default-sc", "other.csi.example.com", true),
			newStorageClass("vi-sc", "csi.example.com", false),
		}, ""),
		Entry("uses default storage class as fallback", func() *v1alpha2.VirtualDisk {
			return newVD("", nil, newDataSource())
		}(), config.VirtualDiskStorageClassSettings{}, []client.Object{
			newVirtualImage(),
			newStorageClass("default-sc", "csi.example.com", true),
			newStorageClass("vi-sc", "csi.example.com", false),
		}, ""),
		Entry("returns clear error when storage class cannot be determined", func() *v1alpha2.VirtualDisk {
			return newVD("", nil, nil)
		}(), config.VirtualDiskStorageClassSettings{}, nil, `storage class for VirtualDisk "target-vd" cannot be determined`),
		Entry("returns readable mismatch error", func() *v1alpha2.VirtualDisk {
			return newVD("", ptr("vd-sc"), newDataSource())
		}(), config.VirtualDiskStorageClassSettings{}, []client.Object{
			newVirtualImage(),
			newStorageClass("vd-sc", "first.csi.example.com", false),
			newStorageClass("vi-sc", "second.csi.example.com", false),
		}, `virtual disk storage class "vd-sc" provisioner does not match virtual image storage class "vi-sc" provisioner`),
	)

	DescribeTable("ValidateUpdate", func(tc updateCase, objs []client.Object, expectedErr string) {
		validator := newValidator(config.VirtualDiskStorageClassSettings{}, objs...)
		_, err := validator.ValidateUpdate(context.Background(), tc.oldVD, tc.newVD)

		if expectedErr == "" {
			Expect(err).NotTo(HaveOccurred())
			return
		}

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(expectedErr))
	},
		Entry("returns nil when data source didn't change", func() updateCase {
			ds := newDataSource()
			return updateCase{
				oldVD: newVD("vd-sc", ptr("vd-sc"), ds),
				newVD: newVD("vd-sc", ptr("vd-sc"), ds),
			}
		}(), nil, ""),
		Entry("skips validation after provisioning is finished", func() updateCase {
			oldVD := newVD("vd-sc", ptr("vd-sc"), nil)
			newVD := newVD("vd-sc", ptr("vd-sc"), newDataSource())
			newVD.Status.Conditions = []metav1.Condition{
				{
					Type:   vdcondition.ReadyType.String(),
					Reason: vdcondition.Ready.String(),
				},
			}
			return updateCase{oldVD: oldVD, newVD: newVD}
		}(), nil, ""),
		Entry("validates and returns mismatch when provisioning is not finished", func() updateCase {
			oldVD := newVD("vd-sc", ptr("vd-sc"), nil)
			newVD := newVD("vd-sc", ptr("vd-sc"), newDataSource())
			newVD.Status.Conditions = []metav1.Condition{
				{
					Type:   vdcondition.ReadyType.String(),
					Reason: vdcondition.Provisioning.String(),
				},
			}
			return updateCase{oldVD: oldVD, newVD: newVD}
		}(), []client.Object{
			newVirtualImage(),
			newStorageClass("vd-sc", "first.csi.example.com", false),
			newStorageClass("vi-sc", "second.csi.example.com", false),
		}, `virtual disk storage class "vd-sc" provisioner does not match virtual image storage class "vi-sc" provisioner`),
		Entry("validates successfully when provisioners are compatible", func() updateCase {
			oldVD := newVD("vd-sc", ptr("vd-sc"), nil)
			newVD := newVD("vd-sc", ptr("vd-sc"), newDataSource())
			return updateCase{oldVD: oldVD, newVD: newVD}
		}(), []client.Object{
			newVirtualImage(),
			newStorageClass("vd-sc", "csi.example.com", false),
			newStorageClass("vi-sc", "csi.example.com", false),
		}, ""),
	)
})
