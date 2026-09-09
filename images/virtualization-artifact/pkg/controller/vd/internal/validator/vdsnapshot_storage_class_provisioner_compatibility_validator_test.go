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

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/config"
	basevc "github.com/deckhouse/virtualization-controller/pkg/controller/service"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("VirtualDiskSnapshotStorageClassValidator", func() {
	const (
		namespace      = "default"
		vdSnapshotName = "source-vdsnapshot"
		vsName         = "source-vs"
		pvcName        = "source-pvc"
		vdName         = "target-vd"
	)

	newScheme := func() *runtime.Scheme {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(vsv1.AddToScheme(scheme)).To(Succeed())
		return scheme
	}

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

	newSnapshotSource := func(snapshotSC string) []client.Object {
		return []client.Object{
			&v1alpha2.VirtualDiskSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vdSnapshotName,
					Namespace: namespace,
				},
				Status: v1alpha2.VirtualDiskSnapshotStatus{
					Phase:              v1alpha2.VirtualDiskSnapshotPhaseReady,
					VolumeSnapshotName: vsName,
					StorageClassName:   snapshotSC,
				},
			},
			&vsv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vsName,
					Namespace: namespace,
				},
				Spec: vsv1.VolumeSnapshotSpec{
					Source: vsv1.VolumeSnapshotSource{
						PersistentVolumeClaimName: ptr.To(pvcName),
					},
				},
			},
			&corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
					Annotations: map[string]string{
						annotations.AnnStorageProvisioner: "nfs.csi.k8s.io",
					},
				},
			},
		}
	}

	newVD := func(statusSC string, specSC *string) *v1alpha2.VirtualDisk {
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vdName,
				Namespace: namespace,
			},
			Spec: v1alpha2.VirtualDiskSpec{
				PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{StorageClass: specSC},
				DataSource: &v1alpha2.VirtualDiskDataSource{
					Type: v1alpha2.DataSourceTypeObjectRef,
					ObjectRef: &v1alpha2.VirtualDiskObjectRef{
						Kind: v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot,
						Name: vdSnapshotName,
					},
				},
			},
		}
		vd.Status.StorageClassName = statusSC
		return vd
	}

	newValidator := func(objs ...client.Object) *VirtualDiskSnapshotStorageClassValidator {
		k8sClient := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(objs...).Build()
		baseSCService := basevc.NewBaseStorageClassService(k8sClient)
		vdSCService := intsvc.NewVirtualDiskStorageClassService(baseSCService, config.VirtualDiskStorageClassSettings{})
		return NewVirtualDiskSnapshotStorageClassValidator(k8sClient, vdSCService)
	}

	DescribeTable("ValidateCreate", func(vd *v1alpha2.VirtualDisk, objs []client.Object, expectedErr string) {
		validator := newValidator(objs...)
		_, err := validator.ValidateCreate(context.Background(), vd)

		if expectedErr == "" {
			Expect(err).NotTo(HaveOccurred())
			return
		}

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(expectedErr))
	},
		Entry("defaults to the snapshot storage class instead of the cluster default",
			newVD("", nil),
			append(newSnapshotSource("nfs"),
				newStorageClass("nfs", "nfs.csi.k8s.io", false),
				newStorageClass("local-thin", "local.csi.storage.deckhouse.io", true),
			), ""),
		Entry("uses status storage class first",
			newVD("nfs", ptr.To("local-thin")),
			append(newSnapshotSource("nfs"),
				newStorageClass("nfs", "nfs.csi.k8s.io", false),
				newStorageClass("local-thin", "local.csi.storage.deckhouse.io", true),
			), ""),
		Entry("rejects explicit spec storage class on a different CSI driver",
			newVD("", ptr.To("local-thin")),
			append(newSnapshotSource("nfs"),
				newStorageClass("nfs", "nfs.csi.k8s.io", false),
				newStorageClass("local-thin", "local.csi.storage.deckhouse.io", true),
			), `virtual disk storage class "local-thin" provisioner "local.csi.storage.deckhouse.io" does not match the source VirtualDiskSnapshot`),
		Entry("falls back to the cluster default when the snapshot has no storage class",
			newVD("", nil),
			append(newSnapshotSource(""),
				newStorageClass("nfs", "nfs.csi.k8s.io", false),
				newStorageClass("local-thin", "local.csi.storage.deckhouse.io", true),
			), `virtual disk storage class "local-thin" provisioner "local.csi.storage.deckhouse.io" does not match the source VirtualDiskSnapshot`),
	)
})
