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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"

	vmbdaBuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmbda"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmbdacondition"
)

var _ = Describe("BlockDeviceReadyHandler ValidateVirtualDiskReady", func() {
	var (
		vmbda                 *v1alpha2.VirtualMachineBlockDeviceAttachment
		cb                    *conditions.ConditionBuilder
		attachmentServiceMock AttachmentServiceMock
		ctx                   context.Context
	)

	BeforeEach(func() {
		vmbda = vmbdaBuilder.NewEmpty("vmbda", "default")
		cb = conditions.NewConditionBuilder(vmbdacondition.BlockDeviceReadyType)
		attachmentServiceMock = AttachmentServiceMock{
			GetVirtualDiskFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
				return nil, nil
			},
			GetPersistentVolumeClaimFunc: func(_ context.Context, _ *service.AttachmentDisk) (*corev1.PersistentVolumeClaim, error) {
				return nil, nil
			},
			GetVirtualMachineFunc: func(_ context.Context, _, _ string) (*v1alpha2.VirtualMachine, error) {
				return nil, nil
			},
			GetKVVMIFunc: func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
				return nil, nil
			},
		}
		ctx = context.Background()
	})

	It("returns error when getting VirtualDisk fails", func() {
		attachmentServiceMock.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
			return nil, errors.New("error")
		}
		err := NewBlockDeviceReadyHandler(&attachmentServiceMock).ValidateVirtualDiskReady(ctx, vmbda, cb)
		Expect(err).To(HaveOccurred())
	})

	It("sets condition to False when VirtualDisk is nil", func() {
		err := NewBlockDeviceReadyHandler(&attachmentServiceMock).ValidateVirtualDiskReady(ctx, vmbda, cb)
		Expect(err).NotTo(HaveOccurred())
		Expect(cb.Condition().Status).To(Equal(metav1.ConditionFalse))
		Expect(cb.Condition().Reason).To(Equal(string(vmbdacondition.BlockDeviceNotReady)))
	})

	It("sets condition to False when VirtualDisk has DeletionTimestamp", func() {
		attachmentServiceMock.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
			return &v1alpha2.VirtualDisk{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &metav1.Time{},
				},
			}, nil
		}
		err := NewBlockDeviceReadyHandler(&attachmentServiceMock).ValidateVirtualDiskReady(ctx, vmbda, cb)
		Expect(err).NotTo(HaveOccurred())
		Expect(cb.Condition().Status).To(Equal(metav1.ConditionFalse))
		Expect(cb.Condition().Reason).To(Equal(string(vmbdacondition.BlockDeviceNotReady)))
	})

	It("returns error when getting PVC fails", func() {
		attachmentServiceMock.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
			return generateVD(v1alpha2.DiskReady, metav1.ConditionTrue), nil
		}
		attachmentServiceMock.GetPersistentVolumeClaimFunc = func(_ context.Context, _ *service.AttachmentDisk) (*corev1.PersistentVolumeClaim, error) {
			return nil, errors.New("error")
		}
		err := NewBlockDeviceReadyHandler(&attachmentServiceMock).ValidateVirtualDiskReady(ctx, vmbda, cb)
		Expect(err).To(HaveOccurred())
	})

	It("sets condition to False when PVC is nil", func() {
		attachmentServiceMock.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
			return generateVD(v1alpha2.DiskReady, metav1.ConditionTrue), nil
		}
		attachmentServiceMock.GetPersistentVolumeClaimFunc = func(_ context.Context, _ *service.AttachmentDisk) (*corev1.PersistentVolumeClaim, error) {
			return nil, nil
		}
		err := NewBlockDeviceReadyHandler(&attachmentServiceMock).ValidateVirtualDiskReady(ctx, vmbda, cb)
		Expect(err).NotTo(HaveOccurred())
		Expect(cb.Condition().Status).To(Equal(metav1.ConditionFalse))
		Expect(cb.Condition().Reason).To(Equal(string(vmbdacondition.BlockDeviceNotReady)))
	})

	It("sets condition to False when VirtualDisk DiskReady condition is False", func() {
		attachmentServiceMock.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
			return generateVD(v1alpha2.DiskReady, metav1.ConditionFalse), nil
		}
		attachmentServiceMock.GetPersistentVolumeClaimFunc = func(_ context.Context, _ *service.AttachmentDisk) (*corev1.PersistentVolumeClaim, error) {
			return nil, nil
		}
		err := NewBlockDeviceReadyHandler(&attachmentServiceMock).ValidateVirtualDiskReady(ctx, vmbda, cb)
		Expect(err).NotTo(HaveOccurred())
		Expect(cb.Condition().Status).To(Equal(metav1.ConditionFalse))
		Expect(cb.Condition().Reason).To(Equal(string(vmbdacondition.BlockDeviceNotReady)))
	})

	DescribeTable("sets condition status based on VirtualDisk phase", func(phase v1alpha2.DiskPhase, expectedStatus metav1.ConditionStatus) {
		attachmentServiceMock.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
			return generateVD(phase, metav1.ConditionTrue), nil
		}
		attachmentServiceMock.GetPersistentVolumeClaimFunc = func(_ context.Context, _ *service.AttachmentDisk) (*corev1.PersistentVolumeClaim, error) {
			return &corev1.PersistentVolumeClaim{
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimBound,
				},
			}, nil
		}
		err := NewBlockDeviceReadyHandler(&attachmentServiceMock).ValidateVirtualDiskReady(ctx, vmbda, cb)
		Expect(err).NotTo(HaveOccurred())
		Expect(cb.Condition().Status).To(Equal(expectedStatus))
		if expectedStatus == metav1.ConditionTrue {
			Expect(cb.Condition().Reason).To(Equal(vmbdacondition.BlockDeviceReady.String()))
		}
	},
		Entry("DiskReady", v1alpha2.DiskReady, metav1.ConditionTrue),
		Entry("DiskMigrating", v1alpha2.DiskMigrating, metav1.ConditionTrue),
		Entry("DiskWaitForFirstConsumer", v1alpha2.DiskWaitForFirstConsumer, metav1.ConditionTrue),
		Entry("DiskPending", v1alpha2.DiskPending, metav1.ConditionFalse),
		Entry("DiskProvisioning", v1alpha2.DiskProvisioning, metav1.ConditionFalse),
		Entry("DiskWaitForUserUpload", v1alpha2.DiskWaitForUserUpload, metav1.ConditionFalse),
		Entry("DiskResizing", v1alpha2.DiskResizing, metav1.ConditionFalse),
		Entry("DiskFailed", v1alpha2.DiskFailed, metav1.ConditionFalse),
		Entry("DiskLost", v1alpha2.DiskLost, metav1.ConditionFalse),
		Entry("DiskExporting", v1alpha2.DiskExporting, metav1.ConditionFalse),
		Entry("DiskTerminating", v1alpha2.DiskTerminating, metav1.ConditionFalse),
	)

	DescribeTable("sets condition status based on VD and PVC phase", func(vdPhase v1alpha2.DiskPhase, pvcPhase corev1.PersistentVolumeClaimPhase, expectedStatus metav1.ConditionStatus) {
		attachmentServiceMock.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
			return generateVD(vdPhase, metav1.ConditionTrue), nil
		}
		attachmentServiceMock.GetPersistentVolumeClaimFunc = func(_ context.Context, _ *service.AttachmentDisk) (*corev1.PersistentVolumeClaim, error) {
			return &corev1.PersistentVolumeClaim{
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: pvcPhase,
				},
			}, nil
		}
		err := NewBlockDeviceReadyHandler(&attachmentServiceMock).ValidateVirtualDiskReady(ctx, vmbda, cb)
		Expect(err).NotTo(HaveOccurred())
		Expect(cb.Condition().Status).To(Equal(expectedStatus))
		if expectedStatus == metav1.ConditionTrue {
			Expect(cb.Condition().Reason).To(Equal(vmbdacondition.BlockDeviceReady.String()))
		}
	},
		Entry("VirtualDisk Ready and PVC ClaimBound", v1alpha2.DiskReady, corev1.ClaimBound, metav1.ConditionTrue),
		Entry("VirtualDisk Ready and PVC ClaimPending", v1alpha2.DiskReady, corev1.ClaimPending, metav1.ConditionFalse),
		Entry("VirtualDisk Ready and PVC ClaimLost", v1alpha2.DiskReady, corev1.ClaimLost, metav1.ConditionFalse),
		Entry("VirtualDisk Migrating and PVC ClaimLost", v1alpha2.DiskMigrating, corev1.ClaimLost, metav1.ConditionFalse),
		Entry("VirtualDisk WaitForFirstConsumer and PVC ClaimLost", v1alpha2.DiskWaitForFirstConsumer, corev1.ClaimLost, metav1.ConditionTrue),
	)

	DescribeTable("sets condition status based on which virtual machine the disk is given to", func(attached []v1alpha2.AttachedVirtualMachine, vmPhase v1alpha2.MachinePhase, phase v1alpha2.BlockDeviceAttachmentPhase, onInstance bool, expectedStatus metav1.ConditionStatus, expectedMessage string) {
		vmbda.Spec.VirtualMachineName = "vm-b"
		vmbda.Status.Phase = phase
		attachmentServiceMock.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
			vd := generateVD(v1alpha2.DiskReady, metav1.ConditionTrue)
			vd.Status.AttachedToVirtualMachines = attached
			return vd, nil
		}
		attachmentServiceMock.GetVirtualMachineFunc = func(_ context.Context, name, ns string) (*v1alpha2.VirtualMachine, error) {
			return &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Status: v1alpha2.VirtualMachineStatus{Phase: vmPhase}}, nil
		}
		attachmentServiceMock.GetKVVMIFunc = func(_ context.Context, _ *v1alpha2.VirtualMachine) (*virtv1.VirtualMachineInstance, error) {
			kvvmi := &virtv1.VirtualMachineInstance{}
			if onInstance {
				kvvmi.Status.VolumeStatus = []virtv1.VolumeStatus{{Name: "vd-vd", HotplugVolume: &virtv1.HotplugVolumeStatus{}}}
			}
			return kvvmi, nil
		}
		attachmentServiceMock.GetPersistentVolumeClaimFunc = func(_ context.Context, _ *service.AttachmentDisk) (*corev1.PersistentVolumeClaim, error) {
			return &corev1.PersistentVolumeClaim{Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}}, nil
		}
		err := NewBlockDeviceReadyHandler(&attachmentServiceMock).ValidateVirtualDiskReady(ctx, vmbda, cb)
		Expect(err).NotTo(HaveOccurred())
		Expect(cb.Condition().Status).To(Equal(expectedStatus))
		Expect(cb.Condition().Message).To(ContainSubstring(expectedMessage))
	},
		Entry("mounted by another virtual machine", []v1alpha2.AttachedVirtualMachine{{Name: "vm-a", Mounted: true}}, v1alpha2.MachineRunning, v1alpha2.BlockDeviceAttachmentPhasePending, false, metav1.ConditionFalse, `"vm-a"`),
		Entry("given to the attachment's virtual machine", []v1alpha2.AttachedVirtualMachine{{Name: "vm-a"}, {Name: "vm-b", Mounted: true}}, v1alpha2.MachineRunning, v1alpha2.BlockDeviceAttachmentPhasePending, false, metav1.ConditionTrue, ""),
		Entry("given to nobody yet while the attachment's virtual machine runs", []v1alpha2.AttachedVirtualMachine{{Name: "vm-a"}}, v1alpha2.MachineRunning, v1alpha2.BlockDeviceAttachmentPhasePending, false, metav1.ConditionFalse, `to be assigned to the VirtualMachine "vm-b"`),
		Entry("given to nobody while the attachment's virtual machine is stopped", []v1alpha2.AttachedVirtualMachine{{Name: "vm-a"}}, v1alpha2.MachineStopped, v1alpha2.BlockDeviceAttachmentPhasePending, false, metav1.ConditionTrue, ""),
		Entry("mounted by another virtual machine, attachment already attached and plugged", []v1alpha2.AttachedVirtualMachine{{Name: "vm-a", Mounted: true}}, v1alpha2.MachineRunning, v1alpha2.BlockDeviceAttachmentPhaseAttached, true, metav1.ConditionTrue, ""),
		Entry("mounted by another virtual machine, attachment attached before a restart", []v1alpha2.AttachedVirtualMachine{{Name: "vm-a", Mounted: true}}, v1alpha2.MachineRunning, v1alpha2.BlockDeviceAttachmentPhaseAttached, false, metav1.ConditionFalse, `"vm-a"`),
	)

	It("sets condition to False while an image is being created from the disk", func() {
		vmbda.Spec.VirtualMachineName = "vm-b"
		vmbda.Status.Phase = v1alpha2.BlockDeviceAttachmentPhasePending
		attachmentServiceMock.GetVirtualDiskFunc = func(_ context.Context, _, _ string) (*v1alpha2.VirtualDisk, error) {
			vd := generateVD(v1alpha2.DiskReady, metav1.ConditionTrue)
			vd.Status.Conditions = append(vd.Status.Conditions, metav1.Condition{
				Type: vdcondition.InUseType.String(), Status: metav1.ConditionTrue, Reason: vdcondition.UsedForImageCreation.String(),
			})
			return vd, nil
		}
		attachmentServiceMock.GetPersistentVolumeClaimFunc = func(_ context.Context, _ *service.AttachmentDisk) (*corev1.PersistentVolumeClaim, error) {
			return &corev1.PersistentVolumeClaim{Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound}}, nil
		}
		err := NewBlockDeviceReadyHandler(&attachmentServiceMock).ValidateVirtualDiskReady(ctx, vmbda, cb)
		Expect(err).NotTo(HaveOccurred())
		Expect(cb.Condition().Status).To(Equal(metav1.ConditionFalse))
	})
})

func generateVD(phase v1alpha2.DiskPhase, readyConditionStatus metav1.ConditionStatus) *v1alpha2.VirtualDisk {
	return &v1alpha2.VirtualDisk{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vd",
			Namespace: "namespace",
		},
		Status: v1alpha2.VirtualDiskStatus{
			Phase: phase,
			Conditions: []metav1.Condition{
				{
					Type:   string(vdcondition.ReadyType),
					Status: readyConditionStatus,
				},
			},
			Target: v1alpha2.DiskTarget{
				PersistentVolumeClaim: "pvc",
			},
		},
	}
}
