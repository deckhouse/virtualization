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

package step

import (
	"context"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	storagefoundationv1alpha1 "github.com/deckhouse/storage-foundation/api/v1alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

// restoreSize reads nothing off the step itself, so a zero value is a complete receiver here.
var sizeStep = CreatePVCFromVDSnapshotStep{}

func vdWithSize(size *resource.Quantity) *v1alpha2.VirtualDisk {
	vd := &v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: "vd"}}
	vd.Spec.PersistentVolumeClaim.Size = size
	return vd
}

// A unified capture keeps both floors in its own status: what the source disk declared, and the size of
// the artifact it was captured into.
func unifiedFloors(declared, artifact string) commonvd.RestoreFloors {
	vds := &v1alpha2.VirtualDiskSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "vds"},
		Status: v1alpha2.VirtualDiskSnapshotStatus{
			CaptureState:              &v1alpha2.UnifiedSnapshotterCaptureState{},
			PersistentVolumeClaimSize: declared,
		},
	}
	if artifact != "" {
		vds.Status.Data = &v1alpha2.UnifiedSnapshotterDataBinding{Size: artifact}
	}

	floors, err := commonvd.RestoreFloorsFrom(vds, nil)
	Expect(err).NotTo(HaveOccurred())
	return floors
}

var _ = Describe("CreatePVCFromVDSnapshotStep sizing a restore", func() {
	It("provisions the size the virtual disk asks for", func() {
		size, err := sizeStep.restoreSize(vdWithSize(ptr.To(resource.MustParse("10Gi"))), unifiedFloors("64Mi", "69580Ki"))
		Expect(err).NotTo(HaveOccurred())
		Expect(size.String()).To(Equal("10Gi"))
	})

	It("refuses a size below what the source disk declared", func() {
		_, err := sizeStep.restoreSize(vdWithSize(ptr.To(resource.MustParse("1Gi"))), unifiedFloors("4Gi", "4Gi"))
		Expect(err).To(MatchError(service.ErrInsufficientPVCSize))
		Expect(err.Error()).To(ContainSubstring("increase spec.persistentVolumeClaim.size to at least 4Gi"))
	})

	// The captured artifact is a floor of the driver's making, not of the user's: growing to it is what
	// keeps a restore of a rounded-up volume working.
	It("grows a request that matches the source disk up to the captured artifact", func() {
		size, err := sizeStep.restoreSize(vdWithSize(ptr.To(resource.MustParse("4Gi"))), unifiedFloors("4Gi", "4100Mi"))
		Expect(err).NotTo(HaveOccurred())
		Expect(size.String()).To(Equal("4100Mi"))
	})

	It("falls back to what the snapshot recorded when no size is asked for", func() {
		size, err := sizeStep.restoreSize(vdWithSize(nil), unifiedFloors("64Mi", ""))
		Expect(err).NotTo(HaveOccurred())
		Expect(size.String()).To(Equal("64Mi"))
	})

	It("grows the fallback up to the captured artifact too", func() {
		size, err := sizeStep.restoreSize(vdWithSize(nil), unifiedFloors("4Gi", "4100Mi"))
		Expect(err).NotTo(HaveOccurred())
		Expect(size.String()).To(Equal("4100Mi"))
	})

	// An import captures nothing, so this module writes neither of its own status fields — only the
	// core's status.data is filled.
	It("falls back to the core's data binding for an imported snapshot", func() {
		size, err := sizeStep.restoreSize(vdWithSize(nil), unifiedFloors("", "69580Ki"))
		Expect(err).NotTo(HaveOccurred())
		Expect(size.String()).To(Equal("69580Ki"))
	})

	// A size the webhook cannot reject any more (it only guards create) must not reach
	// storage-foundation as storage: "0" and provision nothing.
	It("refuses a non-positive size on the disk", func() {
		_, err := sizeStep.restoreSize(vdWithSize(ptr.To(resource.MustParse("0"))), unifiedFloors("64Mi", ""))
		Expect(err).To(MatchError(ContainSubstring("must be greater than 0")))
	})
})

var _ = Describe("CreatePVCFromVDSnapshotStep", func() {
	const (
		vdSnapshotName = "vds"
		vsName         = "vs"
	)

	type createdPVC struct {
		called bool
		size   *resource.Quantity
	}

	newSnapshotScheme := func() *runtime.Scheme {
		scheme := newStepScheme()
		Expect(vsv1.AddToScheme(scheme)).To(Succeed())
		Expect(storagev1.AddToScheme(scheme)).To(Succeed())
		Expect(storagefoundationv1alpha1.AddToScheme(scheme)).To(Succeed())
		return scheme
	}

	newVDFromSnapshot := func(size string) *v1alpha2.VirtualDisk {
		vd := newTestVD(&v1alpha2.VirtualDiskDataSource{
			Type: v1alpha2.DataSourceTypeObjectRef,
			ObjectRef: &v1alpha2.VirtualDiskObjectRef{
				Kind: v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot,
				Name: vdSnapshotName,
			},
		})
		if size != "" {
			vd.Spec.PersistentVolumeClaim.Size = ptr.To(resource.MustParse(size))
		}
		return vd
	}

	newVDSnapshot := func() *v1alpha2.VirtualDiskSnapshot {
		return &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: vdSnapshotName, Namespace: "default"},
			Status: v1alpha2.VirtualDiskSnapshotStatus{
				Phase:              v1alpha2.VirtualDiskSnapshotPhaseReady,
				VolumeSnapshotName: vsName,
			},
		}
	}

	// originalSize is what the source disk requested, restoreSize the floor the CSI driver imposes.
	newVS := func(originalSize, restoreSize string) *vsv1.VolumeSnapshot {
		vs := &vsv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: vsName, Namespace: "default"},
			Status: &vsv1.VolumeSnapshotStatus{
				ReadyToUse: ptr.To(true),
			},
		}
		if originalSize != "" {
			vs.Annotations = map[string]string{annotations.AnnVirtualDiskOriginalSize: originalSize}
		}
		if restoreSize != "" {
			vs.Status.RestoreSize = ptr.To(resource.MustParse(restoreSize))
		}
		return vs
	}

	takeStepWithPVC := func(vd *v1alpha2.VirtualDisk, existingPVC *corev1.PersistentVolumeClaim, objects ...client.Object) (*createdPVC, *metav1.Condition) {
		fakeClient := fake.NewClientBuilder().WithScheme(newSnapshotScheme()).WithObjects(objects...).Build()
		pvc := &createdPVC{}
		pvcSvc := &pvcServiceStub{
			createTargetFromVS: func(_ context.Context, key types.NamespacedName, _ string, size *resource.Quantity, _ client.Object, _ *vsv1.VolumeSnapshot, _ service.VolumeAndAccessModesGetter, _ *provisioner.NodePlacement) (corev1.PersistentVolumeClaim, error) {
				pvc.called = true
				pvc.size = size
				return corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}}, nil
			},
			ensureVolumeRestoreRequest: func(_ context.Context, _ types.NamespacedName, _ string, size *resource.Quantity, _ client.Object, _ storagefoundationv1alpha1.ObjectReference, _ service.VolumeAndAccessModesGetter) error {
				pvc.called = true
				pvc.size = size
				return nil
			},
		}

		cb := conditions.NewConditionBuilder(vdcondition.ReadyType)
		_, err := NewCreatePVCFromVDSnapshotStep(existingPVC, nil, pvcSvc, newTestRecorder(), fakeClient, cb).
			Take(context.Background(), vd)
		Expect(err).ToNot(HaveOccurred())

		conditions.SetCondition(cb, &vd.Status.Conditions)
		ready, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)

		return pvc, &ready
	}

	takeStep := func(vd *v1alpha2.VirtualDisk, objects ...client.Object) (*createdPVC, *metav1.Condition) {
		return takeStepWithPVC(vd, nil, objects...)
	}

	// A PVC created before the size check existed: sized to the source disk, not to the request.
	newExistingPVC := func(phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc", Namespace: "default"},
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("4Gi")},
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{Phase: phase},
		}
	}

	Context("the requested size is less than the size of the source disk", func() {
		It("fails the disk and reports why in the Ready condition", func() {
			vd := newVDFromSnapshot("1Gi")
			pvc, ready := takeStep(vd, newVDSnapshot(), newVS("4Gi", "4Gi"))

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskFailed))
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdcondition.ProvisioningFailed.String()))
			Expect(ready.Message).To(ContainSubstring("pvc size is insufficient"))
			Expect(ready.Message).To(ContainSubstring("1Gi"))
			Expect(ready.Message).To(ContainSubstring("4Gi"))
			Expect(pvc.called).To(BeFalse())
		})
	})

	Context("the restore already has a PVC created before the size check existed", func() {
		It("fails a disk still stuck waiting for that PVC", func() {
			vd := newVDFromSnapshot("1Gi")
			vd.Status.Phase = v1alpha2.DiskWaitForFirstConsumer
			pvc, ready := takeStepWithPVC(vd, newExistingPVC(corev1.ClaimPending), newVDSnapshot(), newVS("4Gi", "4Gi"))

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskFailed))
			Expect(ready.Reason).To(Equal(vdcondition.ProvisioningFailed.String()))
			Expect(ready.Message).To(ContainSubstring("pvc size is insufficient"))
			Expect(pvc.called).To(BeFalse())
		})

		It("leaves a disk whose PVC is already Bound alone", func() {
			vd := newVDFromSnapshot("1Gi")
			vd.Status.Phase = v1alpha2.DiskReady
			pvc, _ := takeStepWithPVC(vd, newExistingPVC(corev1.ClaimBound), newVDSnapshot(), newVS("4Gi", "4Gi"))

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskReady))
			Expect(pvc.called).To(BeFalse())
		})

		It("leaves a disk alone when the requested size is fine", func() {
			vd := newVDFromSnapshot("4Gi")
			vd.Status.Phase = v1alpha2.DiskProvisioning
			pvc, _ := takeStepWithPVC(vd, newExistingPVC(corev1.ClaimPending), newVDSnapshot(), newVS("4Gi", "4Gi"))

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			Expect(pvc.called).To(BeFalse())
		})

		It("leaves a disk alone when the snapshot is gone", func() {
			vd := newVDFromSnapshot("1Gi")
			vd.Status.Phase = v1alpha2.DiskWaitForFirstConsumer
			pvc, _ := takeStepWithPVC(vd, newExistingPVC(corev1.ClaimPending))

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskWaitForFirstConsumer))
			Expect(pvc.called).To(BeFalse())
		})
	})

	Context("the target storage class binds on first consumer", func() {
		// The verdict has to land before the PVC exists: WaitForPVCStep would otherwise park the disk in
		// WaitForFirstConsumer, and the step chain stops at the first step returning a result.
		It("fails the disk without waiting for a consumer", func() {
			vd := newVDFromSnapshot("1Gi")
			vd.Spec.PersistentVolumeClaim.StorageClass = ptr.To("wffc-sc")
			wffcSC := &storagev1.StorageClass{
				ObjectMeta:        metav1.ObjectMeta{Name: "wffc-sc"},
				Provisioner:       "test.csi.storage.deckhouse.io",
				VolumeBindingMode: ptr.To(storagev1.VolumeBindingWaitForFirstConsumer),
			}

			pvc, ready := takeStep(vd, newVDSnapshot(), newVS("4Gi", "4Gi"), wffcSC)

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskFailed))
			Expect(vd.Status.Phase).ToNot(Equal(v1alpha2.DiskWaitForFirstConsumer))
			Expect(ready.Reason).To(Equal(vdcondition.ProvisioningFailed.String()))
			Expect(pvc.called).To(BeFalse())
		})
	})

	Context("the requested size matches the source disk but the driver rounds the snapshot up", func() {
		It("grows the target to the restore size instead of failing", func() {
			vd := newVDFromSnapshot("4Gi")
			pvc, ready := takeStep(vd, newVDSnapshot(), newVS("4Gi", "4100Mi"))

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			Expect(ready.Reason).To(Equal(vdcondition.Provisioning.String()))
			Expect(pvc.called).To(BeTrue())
			Expect(pvc.size.String()).To(Equal("4100Mi"))
		})
	})

	Context("the snapshot carries no original size", func() {
		It("keeps growing the target to the restore size", func() {
			vd := newVDFromSnapshot("1Gi")
			pvc, _ := takeStep(vd, newVDSnapshot(), newVS("", "4Gi"))

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			Expect(pvc.called).To(BeTrue())
			Expect(pvc.size.String()).To(Equal("4Gi"))
		})
	})

	Context("the requested size exceeds both floors", func() {
		It("provisions the requested size", func() {
			vd := newVDFromSnapshot("10Gi")
			pvc, _ := takeStep(vd, newVDSnapshot(), newVS("4Gi", "4Gi"))

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			Expect(pvc.called).To(BeTrue())
			Expect(pvc.size.String()).To(Equal("10Gi"))
		})
	})

	Context("no size is requested at all", func() {
		It("provisions the size of the source disk", func() {
			vd := newVDFromSnapshot("")
			pvc, _ := takeStep(vd, newVDSnapshot(), newVS("4Gi", "4Gi"))

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			Expect(pvc.called).To(BeTrue())
			Expect(pvc.size.String()).To(Equal("4Gi"))
		})
	})

	Context("the disk was failed by an earlier verdict", func() {
		// Raising the size is the documented way out, and nothing in the vd controller gates on Failed.
		It("provisions once the size is raised to the source disk", func() {
			vd := newVDFromSnapshot("4Gi")
			vd.Status.Phase = v1alpha2.DiskFailed
			pvc, ready := takeStep(vd, newVDSnapshot(), newVS("4Gi", "4Gi"))

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			Expect(ready.Reason).To(Equal(vdcondition.Provisioning.String()))
			Expect(pvc.called).To(BeTrue())
			Expect(pvc.size.String()).To(Equal("4Gi"))
		})
	})

	Context("restoring from a unified-snapshotter snapshot", func() {
		newUnifiedVDSnapshot := func(capturedSize string) *v1alpha2.VirtualDiskSnapshot {
			vdSnapshot := newVDSnapshot()
			vdSnapshot.Status.CaptureState = &v1alpha2.UnifiedSnapshotterCaptureState{}
			vdSnapshot.Status.PersistentVolumeClaimSize = capturedSize
			vdSnapshot.Status.Data = &v1alpha2.UnifiedSnapshotterDataBinding{
				ArtifactRef: v1alpha2.UnifiedSnapshotterDataArtifactRef{
					APIVersion: "storage.deckhouse.io/v1alpha1",
					Kind:       "DataArtifact",
					Name:       "artifact",
				},
			}
			return vdSnapshot
		}

		It("fails the disk when the requested size is less than the captured one", func() {
			vd := newVDFromSnapshot("1Gi")
			pvc, ready := takeStep(vd, newUnifiedVDSnapshot("4Gi"))

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskFailed))
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(vdcondition.ProvisioningFailed.String()))
			Expect(ready.Message).To(ContainSubstring("pvc size is insufficient"))
			Expect(pvc.called).To(BeFalse())
		})

		It("fails the disk when neither it nor the snapshot names a size", func() {
			vd := newVDFromSnapshot("")
			vdSnapshot := newUnifiedVDSnapshot("")
			vdSnapshot.Status.Data.Size = ""
			pvc, ready := takeStep(vd, vdSnapshot)

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskFailed))
			Expect(ready.Message).To(ContainSubstring("Cannot determine the size to restore into"))
			Expect(pvc.called).To(BeFalse())
		})

		It("fails a disk still stuck waiting for a PVC created before the size check existed", func() {
			vd := newVDFromSnapshot("1Gi")
			vd.Status.Phase = v1alpha2.DiskWaitForFirstConsumer
			pvc, ready := takeStepWithPVC(vd, newExistingPVC(corev1.ClaimPending), newUnifiedVDSnapshot("4Gi"))

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskFailed))
			Expect(ready.Reason).To(Equal(vdcondition.ProvisioningFailed.String()))
			Expect(pvc.called).To(BeFalse())
		})

		It("provisions the size of the source disk when none is requested", func() {
			vd := newVDFromSnapshot("")
			pvc, _ := takeStep(vd, newUnifiedVDSnapshot("4Gi"))

			Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
			Expect(pvc.called).To(BeTrue())
			Expect(pvc.size.String()).To(Equal("4Gi"))
		})
	})
})

type pvcServiceStub struct {
	createTargetFromVS         func(ctx context.Context, key types.NamespacedName, storageClassName string, size *resource.Quantity, owner client.Object, source *vsv1.VolumeSnapshot, modeGetter service.VolumeAndAccessModesGetter, nodePlacement *provisioner.NodePlacement) (corev1.PersistentVolumeClaim, error)
	ensureVolumeRestoreRequest func(ctx context.Context, key types.NamespacedName, storageClassName string, size *resource.Quantity, owner client.Object, sourceRef storagefoundationv1alpha1.ObjectReference, modeGetter service.VolumeAndAccessModesGetter) error
}

func (s *pvcServiceStub) CreateTargetFromVS(ctx context.Context, key types.NamespacedName, storageClassName string, size *resource.Quantity, owner client.Object, source *vsv1.VolumeSnapshot, modeGetter service.VolumeAndAccessModesGetter, nodePlacement *provisioner.NodePlacement) (corev1.PersistentVolumeClaim, error) {
	return s.createTargetFromVS(ctx, key, storageClassName, size, owner, source, modeGetter, nodePlacement)
}

func (s *pvcServiceStub) EnsureVolumeRestoreRequest(ctx context.Context, key types.NamespacedName, storageClassName string, size *resource.Quantity, owner client.Object, sourceRef storagefoundationv1alpha1.ObjectReference, modeGetter service.VolumeAndAccessModesGetter) error {
	return s.ensureVolumeRestoreRequest(ctx, key, storageClassName, size, owner, sourceRef, modeGetter)
}
