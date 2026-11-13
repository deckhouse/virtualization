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

package internal

import (
	"context"
	"time"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("Deletion handler", func() {
	var (
		snapshotter *DeletionSnapshotterMock
		vdSnapshot  *v1alpha2.VirtualDiskSnapshot
		vs          *vsv1.VolumeSnapshot
	)

	BeforeEach(func() {
		vs = nil
		vdSnapshot = &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vdsnapshot",
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
				Finalizers:        []string{v1alpha2.FinalizerVDSnapshotCleanup},
			},
			Spec: v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: "vd"},
		}

		snapshotter = &DeletionSnapshotterMock{
			GetVolumeSnapshotFunc: func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				return vs, nil
			},
			DeleteVolumeSnapshotFunc: func(_ context.Context, _ *vsv1.VolumeSnapshot) error {
				return nil
			},
		}
	})

	It("deletes the volume snapshot and removes the finalizer", func() {
		vs = &vsv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{Name: "vs"}}

		res, err := NewDeletionHandler(snapshotter).Handle(testContext(), vdSnapshot)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.IsZero()).To(BeTrue())
		Expect(snapshotter.DeleteVolumeSnapshotCalls()).To(HaveLen(1))
		Expect(controllerutil.ContainsFinalizer(vdSnapshot, v1alpha2.FinalizerVDSnapshotCleanup)).To(BeFalse())
	})

	It("removes the finalizer when there is no volume snapshot", func() {
		_, err := NewDeletionHandler(snapshotter).Handle(testContext(), vdSnapshot)
		Expect(err).NotTo(HaveOccurred())
		Expect(snapshotter.DeleteVolumeSnapshotCalls()).To(BeEmpty())
		Expect(controllerutil.ContainsFinalizer(vdSnapshot, v1alpha2.FinalizerVDSnapshotCleanup)).To(BeFalse())
	})

	It("adds the finalizer to a live snapshot", func() {
		vdSnapshot.DeletionTimestamp = nil
		vdSnapshot.Finalizers = nil

		_, err := NewDeletionHandler(snapshotter).Handle(testContext(), vdSnapshot)
		Expect(err).NotTo(HaveOccurred())
		Expect(controllerutil.ContainsFinalizer(vdSnapshot, v1alpha2.FinalizerVDSnapshotCleanup)).To(BeTrue())
	})
})
