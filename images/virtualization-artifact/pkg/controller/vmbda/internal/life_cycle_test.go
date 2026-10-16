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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmbdaBuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

var _ = Describe("LifeCycleHandler Handle", func() {
	var (
		vmbda                 *v1alpha2.VirtualMachineBlockDeviceAttachment
		vm                    *v1alpha2.VirtualMachine
		fakeClient            client.WithWatch
		attachmentServiceMock AttachmentServiceMock
	)

	// migrating returns the Migrating condition as the VM controller sets it: the condition is
	// present with Status=False while the migration is only requested, and with Status=True once
	// it actually runs.
	migrating := func(status metav1.ConditionStatus, reason vmcondition.MigratingReason) []metav1.Condition {
		return []metav1.Condition{
			{
				Type:   vmcondition.TypeMigrating.String(),
				Status: status,
				Reason: reason.String(),
			},
		}
	}

	newVirtualImage := func(storage v1alpha2.StorageType) *v1alpha2.VirtualImage {
		vi := &v1alpha2.VirtualImage{}
		vi.Name = "bd"
		vi.Namespace = "default"
		vi.Spec.Storage = storage
		if storage == v1alpha2.StorageContainerRegistry {
			vi.Status.Target.RegistryURL = "dvcr.example.com/vi/bd"
		} else {
			vi.Status.Target.PersistentVolumeClaim = "pvc"
		}
		return vi
	}

	BeforeEach(func() {
		var err error
		fakeClient, err = testutil.NewFakeClientWithObjects()
		Expect(err).NotTo(HaveOccurred())

		vmbda = vmbdaBuilder.NewEmpty("vmbda", "default")
		vmbdaBuilder.ApplyOptions(vmbda,
			vmbdaBuilder.WithVirtualMachineName("vm"),
			vmbdaBuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, "bd"),
		)
		vmbda.Status.Conditions = []metav1.Condition{
			{Type: vmbdacondition.BlockDeviceReadyType.String(), Status: metav1.ConditionTrue},
			{Type: vmbdacondition.VirtualMachineReadyType.String(), Status: metav1.ConditionTrue},
			{Type: vmbdacondition.DiskAttachmentCapacityAvailableType.String(), Status: metav1.ConditionTrue},
		}

		vm = &v1alpha2.VirtualMachine{}
		vm.Name = "vm"
		vm.Namespace = "default"
		vm.Status.Phase = v1alpha2.MachineRunning

		attachmentServiceMock = AttachmentServiceMock{
			GetVirtualMachineFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
				return vm, nil
			},
			GetKVVMFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachine, error) {
				return &virtv1.VirtualMachine{}, nil
			},
			GetKVVMIFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
				return &virtv1.VirtualMachineInstance{}, nil
			},
			GetVirtualDiskFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				vd := &v1alpha2.VirtualDisk{}
				vd.Name = "bd"
				vd.Namespace = "default"
				vd.Status.Target.PersistentVolumeClaim = "pvc"
				return vd, nil
			},
			GetVirtualImageFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualImage, error) {
				return newVirtualImage(v1alpha2.StoragePersistentVolumeClaim), nil
			},
			GetClusterVirtualImageFunc: func(_ context.Context, _ string) (*v1alpha2.ClusterVirtualImage, error) {
				cvi := &v1alpha2.ClusterVirtualImage{}
				cvi.Name = "bd"
				cvi.Status.Target.RegistryURL = "dvcr.example.com/cvi/bd"
				return cvi, nil
			},
			GetPersistentVolumeClaimFunc: func(_ context.Context, _ *service.AttachmentDisk) (*corev1.PersistentVolumeClaim, error) {
				return &corev1.PersistentVolumeClaim{}, nil
			},
			IsPVAvailableOnVMNodeFunc: func(_ context.Context, _ *corev1.PersistentVolumeClaim, _ *virtv1.VirtualMachineInstance) (bool, error) {
				return true, nil
			},
			IsConflictedAttachmentFunc: func(_ context.Context, _ *v1alpha2.VirtualMachineBlockDeviceAttachment) (bool, string, error) {
				return false, "", nil
			},
			IsHotPluggedFunc: func(_ *service.AttachmentDisk, _ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachineInstance) (bool, error) {
				return false, nil
			},
			CanHotPlugFunc: func(_ *service.AttachmentDisk, _ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) (bool, error) {
				return true, nil
			},
			HotPlugDiskFunc: func(_ context.Context, _ *service.AttachmentDisk, _ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) error {
				return nil
			},
		}
	})

	It("should wait when the disk was given to another virtual machine right before the attachment request", func() {
		vmbdaBuilder.ApplyOptions(vmbda, vmbdaBuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, "bd"))
		attachmentServiceMock.HotPlugDiskFunc = func(_ context.Context, _ *service.AttachmentDisk, _ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) error {
			return fmt.Errorf("%w: virtual disk %q", service.ErrDiskNotGivenToVM, "bd")
		}

		result, err := NewLifeCycleHandler(&attachmentServiceMock, fakeClient).Handle(context.Background(), vmbda)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(vmbda.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhasePending))

		attached, _ := conditions.GetCondition(vmbdacondition.AttachedType, vmbda.Status.Conditions)
		Expect(attached.Reason).To(Equal(vmbdacondition.NotAttached.String()))
	})

	Describe("an earlier attachment of the block device to the same virtual machine", func() {
		volumeName := kvbuilder.GenerateVDDiskName("bd")

		deleted := func(vmName string) *v1alpha2.VirtualMachineBlockDeviceAttachment {
			old := vmbdaBuilder.NewEmpty("vmbda-old", "default")
			vmbdaBuilder.ApplyOptions(old,
				vmbdaBuilder.WithVirtualMachineName(vmName),
				vmbdaBuilder.WithBlockDeviceRef(v1alpha2.VMBDAObjectRefKindVirtualDisk, "bd"),
			)
			old.Finalizers = []string{v1alpha2.FinalizerVMBDACleanup}
			old.DeletionTimestamp = ptr.To(metav1.Now())
			old.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseTerminating
			return old
		}

		withInstance := func(kvvmi *virtv1.VirtualMachineInstance) {
			attachmentServiceMock.GetKVVMIFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
				return kvvmi, nil
			}
		}

		// The instance still reports the volume plugged: a hot-plug check alone would take it as done.
		staleStatus := func(phase virtv1.VolumePhase) *virtv1.VirtualMachineInstance {
			return &virtv1.VirtualMachineInstance{Status: virtv1.VirtualMachineInstanceStatus{
				VolumeStatus: []virtv1.VolumeStatus{{Name: volumeName, HotplugVolume: &virtv1.HotplugVolumeStatus{}, Phase: phase}},
			}}
		}

		expectWaiting := func(messagePart string) {
			GinkgoHelper()
			result, err := NewLifeCycleHandler(&attachmentServiceMock, fakeClient).Handle(context.Background(), vmbda)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
			Expect(vmbda.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhasePending))
			attached, _ := conditions.GetCondition(vmbdacondition.AttachedType, vmbda.Status.Conditions)
			Expect(attached.Reason).To(Equal(vmbdacondition.NotAttached.String()))
			Expect(attached.Message).To(ContainSubstring(messagePart))
			Expect(attachmentServiceMock.IsHotPluggedCalls()).To(BeEmpty())
			Expect(attachmentServiceMock.HotPlugDiskCalls()).To(BeEmpty())
		}

		expectAttachmentRequest := func() {
			GinkgoHelper()
			_, err := NewLifeCycleHandler(&attachmentServiceMock, fakeClient).Handle(context.Background(), vmbda)
			Expect(err).NotTo(HaveOccurred())
			Expect(attachmentServiceMock.HotPlugDiskCalls()).To(HaveLen(1))
			Expect(vmbda.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhaseInProgress))
		}

		It("waits while the deleted attachment unplugs the block device", func() {
			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(deleted("vm"))
			Expect(err).NotTo(HaveOccurred())

			expectWaiting(`VirtualMachineBlockDeviceAttachment "vmbda-old"`)
		})

		It("does not check an attached attachment again", func() {
			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(deleted("vm"))
			Expect(err).NotTo(HaveOccurred())
			vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhaseAttached

			_, err = NewLifeCycleHandler(&attachmentServiceMock, fakeClient).Handle(context.Background(), vmbda)
			Expect(err).NotTo(HaveOccurred())
			attached, _ := conditions.GetCondition(vmbdacondition.AttachedType, vmbda.Status.Conditions)
			Expect(attached.Message).NotTo(ContainSubstring("vmbda-old"))
			Expect(attachmentServiceMock.IsHotPluggedCalls()).NotTo(BeEmpty())
		})

		It("does not wait for a deleted attachment to another virtual machine", func() {
			var err error
			fakeClient, err = testutil.NewFakeClientWithObjects(deleted("vm-old"))
			Expect(err).NotTo(HaveOccurred())

			expectAttachmentRequest()
		})

		It("waits while the instance reports the volume of a finished unplug", func() {
			withInstance(staleStatus(virtv1.VolumeReady))

			expectWaiting("to finish detaching")
		})

		It("does not wait for a volume reported unmounted", func() {
			withInstance(staleStatus(virtv1.HotplugVolumeUnMounted))

			expectAttachmentRequest()
		})

		// An image has no conflict check, so the wait is the only thing that keeps its unplug from undoing the new hot-plug.
		DescribeTable("waits for an earlier unplug of an image",
			func(kind v1alpha2.VMBDAObjectRefKind, staleVolume bool) {
				vmbdaBuilder.ApplyOptions(vmbda, vmbdaBuilder.WithBlockDeviceRef(kind, "bd"))
				if staleVolume {
					name := kvbuilder.GenerateVMBDADiskName(vmbda.Spec.BlockDeviceRef)
					withInstance(&virtv1.VirtualMachineInstance{Status: virtv1.VirtualMachineInstanceStatus{
						VolumeStatus: []virtv1.VolumeStatus{{Name: name, HotplugVolume: &virtv1.HotplugVolumeStatus{}, Phase: virtv1.VolumeReady}},
					}})
					expectWaiting("to finish detaching")
					return
				}

				old := deleted("vm")
				vmbdaBuilder.ApplyOptions(old, vmbdaBuilder.WithBlockDeviceRef(kind, "bd"))
				var err error
				fakeClient, err = testutil.NewFakeClientWithObjects(old)
				Expect(err).NotTo(HaveOccurred())
				expectWaiting(`VirtualMachineBlockDeviceAttachment "vmbda-old"`)
			},
			Entry("virtual image, deleted attachment", v1alpha2.VMBDAObjectRefKindVirtualImage, false),
			Entry("virtual image, volume of a finished unplug", v1alpha2.VMBDAObjectRefKindVirtualImage, true),
			Entry("cluster virtual image, deleted attachment", v1alpha2.VMBDAObjectRefKindClusterVirtualImage, false),
			Entry("cluster virtual image, volume of a finished unplug", v1alpha2.VMBDAObjectRefKindClusterVirtualImage, true),
		)

		It("does not take the volume of its own attachment request for an unplug", func() {
			withInstance(staleStatus(virtv1.HotplugVolumeAttachedToNode))
			attachmentServiceMock.GetKVVMFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachine, error) {
				return &virtv1.VirtualMachine{Status: virtv1.VirtualMachineStatus{
					VolumeRequests: []virtv1.VirtualMachineVolumeRequest{{AddVolumeOptions: &virtv1.AddVolumeOptions{Name: volumeName}}},
				}}, nil
			}
			attachmentServiceMock.CanHotPlugFunc = func(_ *service.AttachmentDisk, _ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) (bool, error) {
				return false, service.ErrHotPlugRequestAlreadySent
			}

			_, err := NewLifeCycleHandler(&attachmentServiceMock, fakeClient).Handle(context.Background(), vmbda)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmbda.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhaseInProgress))
			Expect(attachmentServiceMock.IsHotPluggedCalls()).To(HaveLen(1))
		})
	})

	// Two attachments of one disk to two VMs created in the same second: each one passed the conflict
	// check before the cache showed the other one, and both went on to the hot-plug.
	It("leaves one of two attachments of a disk created at once to different virtual machines", func() {
		newAttachment := func(name, vmName string) *v1alpha2.VirtualMachineBlockDeviceAttachment {
			a := vmbda.DeepCopy()
			a.Name = name
			a.Spec.VirtualMachineName = vmName
			return a
		}
		alpha := newAttachment("vmbda-a", "vm-a")
		beta := newAttachment("vmbda-b", "vm-b")

		// handle runs the handler on a copy of the attachment with the conflict check reading the given cache.
		handle := func(a *v1alpha2.VirtualMachineBlockDeviceAttachment, cache ...*v1alpha2.VirtualMachineBlockDeviceAttachment) *v1alpha2.VirtualMachineBlockDeviceAttachment {
			GinkgoHelper()
			objs := []client.Object{a.DeepCopy()}
			for _, c := range cache {
				objs = append(objs, c.DeepCopy())
			}
			cacheClient, err := testutil.NewFakeClientWithObjects(objs...)
			Expect(err).NotTo(HaveOccurred())
			attachmentServiceMock.IsConflictedAttachmentFunc = service.NewAttachmentService(cacheClient, nil, "").IsConflictedAttachment

			a = a.DeepCopy()
			_, err = NewLifeCycleHandler(&attachmentServiceMock, fakeClient).Handle(context.Background(), a)
			Expect(err).NotTo(HaveOccurred())
			return a
		}

		betaBeforeAlpha := beta
		beta = handle(beta)
		alpha = handle(alpha, betaBeforeAlpha)
		Expect(alpha.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhaseInProgress))
		Expect(beta.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhaseInProgress))

		for range 3 {
			alpha, beta = handle(alpha, beta), handle(beta, alpha)
		}

		Expect(alpha.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhaseInProgress))
		Expect(beta.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhaseFailed))
		attached, _ := conditions.GetCondition(vmbdacondition.AttachedType, beta.Status.Conditions)
		Expect(attached.Reason).To(Equal(vmbdacondition.Conflict.String()))
	})

	DescribeTable("should not send an attachment request while the virtual machine has the Migrating condition", func(kind v1alpha2.VMBDAObjectRefKind, storage v1alpha2.StorageType, status metav1.ConditionStatus, reason vmcondition.MigratingReason) {
		vmbdaBuilder.ApplyOptions(vmbda, vmbdaBuilder.WithBlockDeviceRef(kind, "bd"))
		attachmentServiceMock.GetVirtualImageFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualImage, error) {
			return newVirtualImage(storage), nil
		}
		vm.Status.Conditions = migrating(status, reason)

		result, err := NewLifeCycleHandler(&attachmentServiceMock, fakeClient).Handle(context.Background(), vmbda)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(vmbda.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhasePending))
		Expect(attachmentServiceMock.HotPlugDiskCalls()).To(BeEmpty())

		attached, ok := conditions.GetCondition(vmbdacondition.AttachedType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(attached.Status).To(Equal(metav1.ConditionFalse))
		Expect(attached.Reason).To(Equal(vmbdacondition.BlockedByMigration.String()))
		Expect(attached.Message).To(Equal(fmt.Sprintf(
			"Cannot hot-plug the %s %q while the VirtualMachine %q is migrating. Attachment will continue after the migration completes.",
			kind, "bd", "vm",
		)))
	},
		Entry("virtual disk, migration in progress",
			v1alpha2.VMBDAObjectRefKindVirtualDisk, v1alpha2.StoragePersistentVolumeClaim, metav1.ConditionTrue, vmcondition.ReasonMigratingInProgress),
		Entry("virtual disk, migration is only pending",
			v1alpha2.VMBDAObjectRefKindVirtualDisk, v1alpha2.StoragePersistentVolumeClaim, metav1.ConditionFalse, vmcondition.ReasonMigratingPending),
		Entry("virtual image on a persistent volume claim",
			v1alpha2.VMBDAObjectRefKindVirtualImage, v1alpha2.StoragePersistentVolumeClaim, metav1.ConditionTrue, vmcondition.ReasonMigratingInProgress),
		Entry("virtual image in the container registry",
			v1alpha2.VMBDAObjectRefKindVirtualImage, v1alpha2.StorageContainerRegistry, metav1.ConditionTrue, vmcondition.ReasonMigratingInProgress),
		Entry("cluster virtual image",
			v1alpha2.VMBDAObjectRefKindClusterVirtualImage, v1alpha2.StorageContainerRegistry, metav1.ConditionTrue, vmcondition.ReasonMigratingInProgress),
	)

	DescribeTable("decides on hot-plug by the state of the migrating operation", func(reason vmopcondition.ReasonCompleted, expectBlocked bool) {
		vm.Status.Conditions = migrating(metav1.ConditionFalse, vmcondition.ReasonMigratingPending)
		vmop := &v1alpha2.VirtualMachineOperation{
			ObjectMeta: metav1.ObjectMeta{Name: "vmop", Namespace: "default"},
			Spec: v1alpha2.VirtualMachineOperationSpec{
				Type:           v1alpha2.VMOPTypeMigrate,
				VirtualMachine: "vm",
			},
			Status: v1alpha2.VirtualMachineOperationStatus{
				Phase: v1alpha2.VMOPPhasePending,
				Conditions: []metav1.Condition{
					{
						Type:   vmopcondition.TypeCompleted.String(),
						Status: metav1.ConditionFalse,
						Reason: reason.String(),
					},
				},
			},
		}

		var err error
		fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
		Expect(err).NotTo(HaveOccurred())

		result, err := NewLifeCycleHandler(&attachmentServiceMock, fakeClient).Handle(context.Background(), vmbda)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		attached, ok := conditions.GetCondition(vmbdacondition.AttachedType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())

		if expectBlocked {
			Expect(vmbda.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhasePending))
			Expect(attached.Reason).To(Equal(vmbdacondition.BlockedByMigration.String()))
			Expect(attachmentServiceMock.HotPlugDiskCalls()).To(BeEmpty())
		} else {
			Expect(vmbda.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhaseInProgress))
			Expect(attached.Reason).To(Equal(vmbdacondition.AttachmentRequestSent.String()))
			Expect(attachmentServiceMock.HotPlugDiskCalls()).To(HaveLen(1))
		}
	},
		Entry("parked on the project quota", vmopcondition.ReasonQuotaExceeded, false),
		Entry("waiting for another hot-plug request", vmopcondition.ReasonWaitingForBlockDeviceAttachment, false),
		Entry("target is being scheduled", vmopcondition.ReasonTargetScheduling, true),
		Entry("waiting for a sync slot on a prepared target", vmopcondition.ReasonWaitingForSyncSlot, true),
		Entry("migration is running", vmopcondition.ReasonMigrationRunning, true),
	)

	DescribeTable("treats a migrating operation without the Completed condition by its phase", func(phase v1alpha2.VMOPPhase, expectBlocked bool) {
		vm.Status.Conditions = migrating(metav1.ConditionFalse, vmcondition.ReasonMigratingPending)
		vmop := &v1alpha2.VirtualMachineOperation{
			ObjectMeta: metav1.ObjectMeta{Name: "vmop", Namespace: "default"},
			Spec: v1alpha2.VirtualMachineOperationSpec{
				Type:           v1alpha2.VMOPTypeMigrate,
				VirtualMachine: "vm",
			},
			Status: v1alpha2.VirtualMachineOperationStatus{Phase: phase},
		}

		var err error
		fakeClient, err = testutil.NewFakeClientWithObjects(vmop)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewLifeCycleHandler(&attachmentServiceMock, fakeClient).Handle(context.Background(), vmbda)
		Expect(err).NotTo(HaveOccurred())

		attached, _ := conditions.GetCondition(vmbdacondition.AttachedType, vmbda.Status.Conditions)
		if expectBlocked {
			Expect(attached.Reason).To(Equal(vmbdacondition.BlockedByMigration.String()))
			Expect(attachmentServiceMock.HotPlugDiskCalls()).To(BeEmpty())
		} else {
			Expect(attached.Reason).To(Equal(vmbdacondition.AttachmentRequestSent.String()))
			Expect(attachmentServiceMock.HotPlugDiskCalls()).To(HaveLen(1))
		}
	},
		Entry("still queued", v1alpha2.VMOPPhasePending, false),
		Entry("already in progress", v1alpha2.VMOPPhaseInProgress, true),
		Entry("phase not set yet", v1alpha2.VMOPPhase(""), true),
	)

	It("should send an attachment request if the virtual machine has no Migrating condition", func() {
		result, err := NewLifeCycleHandler(&attachmentServiceMock, fakeClient).Handle(context.Background(), vmbda)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(vmbda.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhaseInProgress))
		Expect(attachmentServiceMock.HotPlugDiskCalls()).To(HaveLen(1))

		attached, ok := conditions.GetCondition(vmbdacondition.AttachedType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(attached.Reason).To(Equal(vmbdacondition.AttachmentRequestSent.String()))
	})

	It("should keep an already hot-plugged block device attached while the virtual machine is migrating", func() {
		vm.Status.Conditions = migrating(metav1.ConditionTrue, vmcondition.ReasonMigratingInProgress)
		attachmentServiceMock.IsHotPluggedFunc = func(_ *service.AttachmentDisk, _ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachineInstance) (bool, error) {
			return true, nil
		}

		result, err := NewLifeCycleHandler(&attachmentServiceMock, fakeClient).Handle(context.Background(), vmbda)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(vmbda.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhaseAttached))

		attached, ok := conditions.GetCondition(vmbdacondition.AttachedType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(attached.Status).To(Equal(metav1.ConditionTrue))
		Expect(attached.Reason).To(Equal(vmbdacondition.Attached.String()))
	})

	It("should send the attachment request once the Migrating condition is gone", func() {
		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhasePending
		conditions.SetCondition(
			conditions.NewConditionBuilder(vmbdacondition.AttachedType).
				Status(metav1.ConditionFalse).
				Reason(vmbdacondition.BlockedByMigration),
			&vmbda.Status.Conditions)

		result, err := NewLifeCycleHandler(&attachmentServiceMock, fakeClient).Handle(context.Background(), vmbda)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(vmbda.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhaseInProgress))
		Expect(attachmentServiceMock.HotPlugDiskCalls()).To(HaveLen(1))

		attached, ok := conditions.GetCondition(vmbdacondition.AttachedType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(attached.Reason).To(Equal(vmbdacondition.AttachmentRequestSent.String()))
	})

	It("should stay in progress when the migration starts after the attachment request was sent", func() {
		vm.Status.Conditions = migrating(metav1.ConditionTrue, vmcondition.ReasonMigratingInProgress)
		attachmentServiceMock.CanHotPlugFunc = func(_ *service.AttachmentDisk, _ *v1alpha2.VirtualMachine, _ *virtv1.VirtualMachine) (bool, error) {
			return false, service.ErrHotPlugRequestAlreadySent
		}

		result, err := NewLifeCycleHandler(&attachmentServiceMock, fakeClient).Handle(context.Background(), vmbda)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
		Expect(vmbda.Status.Phase).To(Equal(v1alpha2.BlockDeviceAttachmentPhaseInProgress))
		Expect(attachmentServiceMock.HotPlugDiskCalls()).To(BeEmpty())

		attached, ok := conditions.GetCondition(vmbdacondition.AttachedType, vmbda.Status.Conditions)
		Expect(ok).To(BeTrue())
		Expect(attached.Reason).To(Equal(vmbdacondition.AttachmentRequestSent.String()))
	})
})
