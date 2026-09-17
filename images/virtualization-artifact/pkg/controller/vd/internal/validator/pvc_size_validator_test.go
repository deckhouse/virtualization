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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// The rejection has to use the same floor the provisioning step fails on: a target the step would grow
// to the driver's restoreSize is a supported restore, and refusing it would block restores that have
// always worked.
var _ = Describe("PVCSizeValidator restoring from a snapshot", func() {
	const (
		namespace      = "default"
		vdSnapshotName = "vds"
		vsName         = "vs"
	)

	newScheme := func() *runtime.Scheme {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(vsv1.AddToScheme(scheme)).To(Succeed())
		return scheme
	}

	newVD := func(size string) *v1alpha2.VirtualDisk {
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd", Namespace: namespace},
			Spec: v1alpha2.VirtualDiskSpec{
				DataSource: &v1alpha2.VirtualDiskDataSource{
					Type: v1alpha2.DataSourceTypeObjectRef,
					ObjectRef: &v1alpha2.VirtualDiskObjectRef{
						Kind: v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot,
						Name: vdSnapshotName,
					},
				},
			},
		}
		if size != "" {
			vd.Spec.PersistentVolumeClaim.Size = ptr.To(resource.MustParse(size))
		}
		return vd
	}

	newCSISource := func(originalSize, restoreSize string) []client.Object {
		vs := &vsv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: vsName, Namespace: namespace},
			Status:     &vsv1.VolumeSnapshotStatus{RestoreSize: ptr.To(resource.MustParse(restoreSize))},
		}
		if originalSize != "" {
			vs.Annotations = map[string]string{annotations.AnnVirtualDiskOriginalSize: originalSize}
		}
		return []client.Object{
			&v1alpha2.VirtualDiskSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: vdSnapshotName, Namespace: namespace},
				Status: v1alpha2.VirtualDiskSnapshotStatus{
					Phase:              v1alpha2.VirtualDiskSnapshotPhaseReady,
					VolumeSnapshotName: vsName,
				},
			},
			vs,
		}
	}

	newUnifiedSource := func(capturedSize string) []client.Object {
		return []client.Object{
			&v1alpha2.VirtualDiskSnapshot{
				ObjectMeta: metav1.ObjectMeta{Name: vdSnapshotName, Namespace: namespace},
				Status: v1alpha2.VirtualDiskSnapshotStatus{
					Phase:                     v1alpha2.VirtualDiskSnapshotPhaseReady,
					CaptureState:              &v1alpha2.UnifiedSnapshotterCaptureState{},
					PersistentVolumeClaimSize: capturedSize,
				},
			},
		}
	}

	validate := func(vd *v1alpha2.VirtualDisk, objects []client.Object) error {
		fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(objects...).Build()
		warnings, err := NewPVCSizeValidator(fakeClient).ValidateCreate(context.Background(), vd)
		Expect(warnings).To(BeEmpty())
		return err
	}

	It("rejects a target smaller than the source disk", func() {
		err := validate(newVD("1Gi"), newCSISource("4Gi", "4Gi"))

		Expect(err).To(MatchError(service.ErrInsufficientPVCSize))
		Expect(err.Error()).To(ContainSubstring("increase spec.persistentVolumeClaim.size to at least 4Gi"))
	})

	It("admits a target the step grows to the driver floor", func() {
		Expect(validate(newVD("4Gi"), newCSISource("4Gi", "4100Mi"))).To(Succeed())
	})

	It("admits a restore whose snapshot records no size of the source disk", func() {
		Expect(validate(newVD("1Gi"), newCSISource("", "4Gi"))).To(Succeed())
	})

	It("rejects a restore from a unified snapshot the same way", func() {
		err := validate(newVD("1Gi"), newUnifiedSource("4Gi"))

		Expect(err).To(MatchError(ContainSubstring(`the VirtualDiskSnapshot "vds" was taken from a 4Gi disk`)))
	})

	It("admits a restore that requests no size at all", func() {
		Expect(validate(newVD(""), newUnifiedSource("4Gi"))).To(Succeed())
	})
})
