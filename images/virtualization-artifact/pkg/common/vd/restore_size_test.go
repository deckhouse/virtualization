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

package vd

import (
	"context"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

var _ = Describe("RestoreFloors", func() {
	const namespace = "default"

	newScheme := func() *runtime.Scheme {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(vsv1.AddToScheme(scheme)).To(Succeed())
		return scheme
	}

	builtInSnapshot := func() *v1alpha2.VirtualDiskSnapshot {
		return &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vds", Namespace: namespace},
			Status:     v1alpha2.VirtualDiskSnapshotStatus{VolumeSnapshotName: "vs"},
		}
	}

	unifiedSnapshot := func(declared, artifact string) *v1alpha2.VirtualDiskSnapshot {
		vds := &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{Name: "vds", Namespace: namespace},
			Status: v1alpha2.VirtualDiskSnapshotStatus{
				CaptureState:              &v1alpha2.UnifiedSnapshotterCaptureState{},
				PersistentVolumeClaimSize: declared,
			},
		}
		if artifact != "" {
			vds.Status.Data = &v1alpha2.UnifiedSnapshotterDataBinding{Size: artifact}
		}
		return vds
	}

	volumeSnapshot := func(declared, restoreSize string) *vsv1.VolumeSnapshot {
		vs := &vsv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{Name: "vs", Namespace: namespace}}
		if declared != "" {
			vs.Annotations = map[string]string{annotations.AnnVirtualDiskOriginalSize: declared}
		}
		if restoreSize != "" {
			vs.Status = &vsv1.VolumeSnapshotStatus{RestoreSize: ptr.To(resource.MustParse(restoreSize))}
		}
		return vs
	}

	resolve := func(vds *v1alpha2.VirtualDiskSnapshot, objects ...client.Object) RestoreFloors {
		fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(objects...).Build()
		floors, err := ResolveRestoreFloors(context.Background(), fakeClient, vds)
		Expect(err).NotTo(HaveOccurred())
		return floors
	}

	// The two mechanisms record the same two numbers in different places; everything downstream reads
	// them through here, so this is where the mapping has to be right.
	It("reads both floors off a unified capture", func() {
		floors := resolve(unifiedSnapshot("4Gi", "4100Mi"))

		Expect(floors.Declared.String()).To(Equal("4Gi"))
		Expect(floors.Driver.String()).To(Equal("4100Mi"))
	})

	It("reads both floors off the CSI VolumeSnapshot", func() {
		floors := resolve(builtInSnapshot(), volumeSnapshot("4Gi", "4100Mi"))

		Expect(floors.Declared.String()).To(Equal("4Gi"))
		Expect(floors.Driver.String()).To(Equal("4100Mi"))
	})

	It("leaves the floors unset when the snapshot records neither", func() {
		floors := resolve(builtInSnapshot(), volumeSnapshot("", ""))

		Expect(floors.Declared).To(BeNil())
		Expect(floors.Driver).To(BeNil())
	})

	It("leaves the floors unset when the VolumeSnapshot is gone", func() {
		floors := resolve(builtInSnapshot())

		Expect(floors.Declared).To(BeNil())
		Expect(floors.Driver).To(BeNil())
	})

	It("reports a size it cannot parse", func() {
		fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).Build()
		_, err := ResolveRestoreFloors(context.Background(), fakeClient, unifiedSnapshot("not-a-size", ""))

		Expect(err).To(MatchError(ContainSubstring("parse the captured PVC size")))
	})

	Describe("Validate", func() {
		It("refuses less than the source disk declared", func() {
			err := resolve(unifiedSnapshot("4Gi", "4Gi")).Validate(ptr.To(resource.MustParse("1Gi")))

			Expect(err).To(MatchError(service.ErrInsufficientPVCSize))
			Expect(err.Error()).To(ContainSubstring(`the VirtualDiskSnapshot "vds" was taken from a 4Gi disk`))
		})

		It("admits the declared size itself", func() {
			Expect(resolve(unifiedSnapshot("4Gi", "4Gi")).Validate(ptr.To(resource.MustParse("4Gi")))).To(Succeed())
		})

		// The driver floor is none of the user's doing: a request above the declared size is grown to it,
		// never refused.
		It("admits a size between the two floors", func() {
			Expect(resolve(unifiedSnapshot("4Gi", "4100Mi")).Validate(ptr.To(resource.MustParse("4Gi")))).To(Succeed())
		})

		It("admits anything when the snapshot records no declared size", func() {
			Expect(resolve(unifiedSnapshot("", "4Gi")).Validate(ptr.To(resource.MustParse("1Gi")))).To(Succeed())
		})

		It("admits a restore that asks for no size", func() {
			Expect(resolve(unifiedSnapshot("4Gi", "4Gi")).Validate(nil)).To(Succeed())
		})
	})

	Describe("Target", func() {
		It("keeps a request that clears both floors", func() {
			Expect(resolve(unifiedSnapshot("4Gi", "4100Mi")).Target(ptr.To(resource.MustParse("10Gi"))).String()).To(Equal("10Gi"))
		})

		It("grows a request up to the driver floor", func() {
			Expect(resolve(unifiedSnapshot("4Gi", "4100Mi")).Target(ptr.To(resource.MustParse("4Gi"))).String()).To(Equal("4100Mi"))
		})

		It("falls back to the declared size, grown the same way", func() {
			Expect(resolve(unifiedSnapshot("4Gi", "4100Mi")).Target(nil).String()).To(Equal("4100Mi"))
			Expect(resolve(unifiedSnapshot("4Gi", "")).Target(nil).String()).To(Equal("4Gi"))
		})

		It("falls back to the driver floor for an imported snapshot", func() {
			Expect(resolve(unifiedSnapshot("", "69580Ki")).Target(nil).String()).To(Equal("69580Ki"))
		})

		It("has nothing to offer when the snapshot records neither floor", func() {
			Expect(resolve(builtInSnapshot(), volumeSnapshot("", "")).Target(nil)).To(BeNil())
		})
	})
})
