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
	"fmt"
	"time"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("Deletion handler", func() {
	var (
		snapshotter *DeletionSnapshotterMock
		vdSnapshot  *v1alpha2.VirtualDiskSnapshot
		kvvmi       *virtv1.VirtualMachineInstance
		syncErr     error
		canUnfreeze bool
	)

	errInFlight := fmt.Errorf("request: freeze: %w", service.ErrUntrustedFilesystemFrozenCondition)

	BeforeEach(func() {
		syncErr = nil
		canUnfreeze = false

		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd"},
			Status: v1alpha2.VirtualDiskStatus{
				AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{{Name: "vm"}},
			},
		}
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm"}}
		kvvmi = &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "vm"},
			Status:     virtv1.VirtualMachineInstanceStatus{Phase: virtv1.Running},
		}

		vdSnapshot = &v1alpha2.VirtualDiskSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vdsnapshot",
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
				Finalizers:        []string{v1alpha2.FinalizerVDSnapshotCleanup},
			},
			Spec: v1alpha2.VirtualDiskSnapshotSpec{VirtualDiskName: vd.Name},
		}

		snapshotter = &DeletionSnapshotterMock{
			GetVolumeSnapshotFunc: func(_ context.Context, _, _ string) (*vsv1.VolumeSnapshot, error) {
				return nil, nil
			},
			GetVirtualDiskFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				return vd, nil
			},
			GetVirtualMachineFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
				return vm, nil
			},
			GetVirtualMachineInstanceFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
				return kvvmi, nil
			},
			SyncFSFreezeRequestFunc: func(_ context.Context, _ *virtv1.VirtualMachineInstance) error {
				return syncErr
			},
			DiscardFSFreezeRequestFunc: func(_ context.Context, _ *virtv1.VirtualMachineInstance) error {
				return nil
			},
			CanUnfreezeWithVirtualDiskSnapshotFunc: func(_ context.Context, _ string, _ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachineInstance) (bool, error) {
				return canUnfreeze, nil
			},
			UnfreezeFunc: func(_ context.Context, _ *virtv1.VirtualMachineInstance) error {
				return nil
			},
		}
	})

	It("waits for an in-flight filesystem request instead of dropping the reconcile", func() {
		syncErr = errInFlight

		res, err := NewDeletionHandler(snapshotter).Handle(testContext(), vdSnapshot)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.RequeueAfter).To(BeNumerically(">", 0))
		Expect(controllerutil.ContainsFinalizer(vdSnapshot, v1alpha2.FinalizerVDSnapshotCleanup)).To(BeTrue())
		Expect(snapshotter.DiscardFSFreezeRequestCalls()).To(BeEmpty())
		Expect(snapshotter.UnfreezeCalls()).To(BeEmpty())
	})

	It("discards an unconfirmed filesystem request past the deadline and unfreezes", func() {
		syncErr = errInFlight
		canUnfreeze = true
		vdSnapshot.DeletionTimestamp = &metav1.Time{Time: time.Now().Add(-2 * fsRequestConfirmDeadline)}

		res, err := NewDeletionHandler(snapshotter).Handle(testContext(), vdSnapshot)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.IsZero()).To(BeTrue())
		Expect(snapshotter.DiscardFSFreezeRequestCalls()).To(HaveLen(1))
		Expect(snapshotter.UnfreezeCalls()).To(HaveLen(1))
		Expect(controllerutil.ContainsFinalizer(vdSnapshot, v1alpha2.FinalizerVDSnapshotCleanup)).To(BeFalse())
	})

	It("unfreezes a frozen guest and removes the finalizer", func() {
		canUnfreeze = true

		res, err := NewDeletionHandler(snapshotter).Handle(testContext(), vdSnapshot)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.IsZero()).To(BeTrue())
		Expect(snapshotter.SyncFSFreezeRequestCalls()).To(HaveLen(1))
		Expect(snapshotter.UnfreezeCalls()).To(HaveLen(1))
		Expect(controllerutil.ContainsFinalizer(vdSnapshot, v1alpha2.FinalizerVDSnapshotCleanup)).To(BeFalse())
	})

	It("removes the finalizer once a settled request leaves nothing to unfreeze", func() {
		res, err := NewDeletionHandler(snapshotter).Handle(testContext(), vdSnapshot)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.IsZero()).To(BeTrue())
		Expect(snapshotter.UnfreezeCalls()).To(BeEmpty())
		Expect(controllerutil.ContainsFinalizer(vdSnapshot, v1alpha2.FinalizerVDSnapshotCleanup)).To(BeFalse())
	})

	It("removes the finalizer when the virtual machine has no instance", func() {
		kvvmi = nil

		_, err := NewDeletionHandler(snapshotter).Handle(testContext(), vdSnapshot)
		Expect(err).NotTo(HaveOccurred())
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
