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

package vi

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

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/config"
	basevc "github.com/deckhouse/virtualization-controller/pkg/controller/service"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("Validator source storage class provisioner compatibility", func() {
	const (
		namespace      = "default"
		vdSnapshotName = "source-vdsnapshot"
		vsName         = "source-vs"
		pvcName        = "source-pvc"
	)

	newObjects := func(vsSC string) []client.Object {
		vsAnnotations := map[string]string{}
		if vsSC != "" {
			vsAnnotations[annotations.AnnStorageClassName] = vsSC
		}
		return []client.Object{
			&v1alpha2.VirtualDiskSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: vdSnapshotName, Namespace: namespace},
				Status: v1alpha2.VirtualDiskSnapshotStatus{
					Phase:              v1alpha2.VirtualDiskSnapshotPhaseReady,
					VolumeSnapshotName: vsName,
				},
			},
			&vsv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: vsName, Namespace: namespace, Annotations: vsAnnotations},
				Spec: vsv1.VolumeSnapshotSpec{
					Source: vsv1.VolumeSnapshotSource{PersistentVolumeClaimName: ptr.To(pvcName)},
				},
			},
			&corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:        pvcName,
					Namespace:   namespace,
					Annotations: map[string]string{annotations.AnnStorageProvisioner: "nfs.csi.k8s.io"},
				},
			},
			&storagev1.StorageClass{
				ObjectMeta:  metav1.ObjectMeta{Name: "nfs"},
				Provisioner: "nfs.csi.k8s.io",
			},
			&storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "local-thin",
					Annotations: map[string]string{annotations.AnnDefaultStorageClass: "true"},
				},
				Provisioner: "local.csi.storage.deckhouse.io",
			},
		}
	}

	newVI := func(specSC *string) *v1alpha2.VirtualImage {
		return &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{Name: "target-vi", Namespace: namespace},
			Spec: v1alpha2.VirtualImageSpec{
				Storage:               v1alpha2.StoragePersistentVolumeClaim,
				PersistentVolumeClaim: v1alpha2.VirtualImagePersistentVolumeClaim{StorageClass: specSC},
				DataSource: v1alpha2.VirtualImageDataSource{
					Type: v1alpha2.DataSourceTypeObjectRef,
					ObjectRef: &v1alpha2.VirtualImageObjectRef{
						Kind: v1alpha2.VirtualImageObjectRefKindVirtualDiskSnapshot,
						Name: vdSnapshotName,
					},
				},
			},
		}
	}

	DescribeTable("validateSourceStorageClassProvisionerCompatibility", func(vi *v1alpha2.VirtualImage, vsSC, expectedErr string) {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(vsv1.AddToScheme(scheme)).To(Succeed())
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(newObjects(vsSC)...).Build()
		scService := intsvc.NewVirtualImageStorageClassService(basevc.NewBaseStorageClassService(k8sClient), config.VirtualImageStorageClassSettings{})
		validator := NewValidator(log.NewNop(), k8sClient, scService)

		err := validator.validateSourceStorageClassProvisionerCompatibility(context.Background(), vi)

		if expectedErr == "" {
			Expect(err).NotTo(HaveOccurred())
			return
		}
		Expect(err).To(MatchError(ContainSubstring(expectedErr)))
	},
		Entry("defaults to the storage class captured on the VolumeSnapshot", newVI(nil), "nfs", ""),
		Entry("rejects explicit spec storage class on a different CSI driver", newVI(ptr.To("local-thin")), "nfs",
			`virtual image storage class "local-thin" provisioner "local.csi.storage.deckhouse.io" does not match`),
		Entry("falls back to the cluster default when the snapshot has no storage class", newVI(nil), "",
			`virtual image storage class "local-thin" provisioner "local.csi.storage.deckhouse.io" does not match`),
	)
})
