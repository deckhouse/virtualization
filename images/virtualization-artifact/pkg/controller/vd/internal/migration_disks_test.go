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
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("MigrationHandler with disks of the same migration", func() {
	const (
		namespace = "default"
		vmName    = "test-vm"
	)

	var (
		ctx            context.Context
		log            *slog.Logger
		fakeClient     client.Client
		handler        *MigrationHandler
		migrationStart time.Time
	)

	newDisk := func(name, reason string) *v1alpha2.VirtualDisk {
		return &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: types.UID(name)},
			Status: v1alpha2.VirtualDiskStatus{
				Target: v1alpha2.DiskTarget{PersistentVolumeClaim: name + "-source"},
				Conditions: []metav1.Condition{
					{
						Type:   vdcondition.InUseType.String(),
						Status: metav1.ConditionTrue,
						Reason: vdcondition.AttachedToVirtualMachine.String(),
					},
					{
						Type:   vdcondition.MigratingType.String(),
						Status: metav1.ConditionTrue,
						Reason: reason,
					},
				},
				AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{{Name: vmName, Mounted: true}},
				MigrationState: v1alpha2.VirtualDiskMigrationState{
					SourcePVC:      name + "-source",
					TargetPVC:      name + "-target",
					StartTimestamp: metav1.NewTime(migrationStart),
				},
			},
		}
	}

	finish := func(vd *v1alpha2.VirtualDisk, result v1alpha2.VirtualDiskMigrationResult, end time.Time) *v1alpha2.VirtualDisk {
		conditions.RemoveCondition(vdcondition.MigratingType, &vd.Status.Conditions)
		vd.Status.MigrationState.Result = result
		vd.Status.MigrationState.EndTimestamp = metav1.NewTime(end)
		return vd
	}

	newPVC := func(name string, owner *v1alpha2.VirtualDisk) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       namespace,
				OwnerReferences: []metav1.OwnerReference{{UID: owner.UID}},
			},
			Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		}
	}

	pvcExists := func(name string) bool {
		err := fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &corev1.PersistentVolumeClaim{})
		return err == nil
	}

	BeforeEach(func() {
		ctx = testutil.ContextBackgroundWithNoOpLogger()
		log = logger.FromContext(ctx)
		migrationStart = time.Now().Add(-time.Minute).Truncate(time.Second)

		scheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(virtv1.AddToScheme(scheme)).To(Succeed())

		// The restarted VirtualMachine: no migration state, not running.
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: v1alpha2.MachineStarting,
				BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
					{Kind: v1alpha2.DiskDevice, Name: "disk-a"},
					{Kind: v1alpha2.DiskDevice, Name: "disk-b"},
				},
				Conditions: []metav1.Condition{{
					Type:   vmcondition.TypeRunning.String(),
					Status: metav1.ConditionFalse,
					Reason: vmcondition.ReasonPodNotStarted.String(),
				}},
			},
		}
		kvvm := &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace},
			Spec:       virtv1.VirtualMachineSpec{Template: &virtv1.VirtualMachineInstanceTemplateSpec{}},
		}
		for _, name := range []string{"disk-a", "disk-b", "disk-c"} {
			kvvm.Spec.Template.Spec.Volumes = append(kvvm.Spec.Template.Spec.Volumes, virtv1.Volume{
				Name: "vd-" + name,
				VolumeSource: virtv1.VolumeSource{PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: name + "-target"},
				}},
			})
		}
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, kvvm).Build()

		recorder := &eventrecord.EventRecorderLoggerMock{
			EventFunc:  func(client.Object, string, string, string) {},
			EventfFunc: func(client.Object, string, string, string, ...interface{}) {},
		}
		handler = NewMigrationHandler(fakeClient, fakeClient, recorder, &fakeStorageClassValidator{}, &fakeVolumeAndAccessModesGetter{}, featuregates.Default())
	})

	Describe("getAction after the VirtualMachine restart", func() {
		It("should keep completing a disk that waits for the source release", func() {
			vd := newDisk("disk-a", vdcondition.MigratingWaitForSourceVolumeReleaseReason.String())

			action, err := handler.getAction(ctx, vd, log)
			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(complete))
		})

		It("should revert a disk when no disk of the same migration completes", func() {
			vd := newDisk("disk-a", vdcondition.MigratingInProgressReason.String())
			Expect(fakeClient.Create(ctx, newDisk("disk-b", vdcondition.MigratingInProgressReason.String()))).To(Succeed())

			action, err := handler.getAction(ctx, vd, log)
			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(revert))
		})

		DescribeTable("should complete a disk whose migration is completed by another disk",
			func(other func() *v1alpha2.VirtualDisk) {
				vd := newDisk("disk-a", vdcondition.MigratingInProgressReason.String())
				Expect(fakeClient.Create(ctx, other())).To(Succeed())

				action, err := handler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(complete))
			},
			Entry("waits for the source release", func() *v1alpha2.VirtualDisk {
				return newDisk("disk-b", vdcondition.MigratingWaitForSourceVolumeReleaseReason.String())
			}),
			Entry("completed", func() *v1alpha2.VirtualDisk {
				return finish(newDisk("disk-b", ""), v1alpha2.VirtualDiskMigrationResultSucceeded, time.Now())
			}),
		)

		It("should not follow a disk completed in the previous migration", func() {
			vd := newDisk("disk-a", vdcondition.MigratingInProgressReason.String())
			other := finish(newDisk("disk-b", ""), v1alpha2.VirtualDiskMigrationResultSucceeded, migrationStart.Add(-5*time.Minute))
			other.Status.MigrationState.StartTimestamp = metav1.NewTime(migrationStart.Add(-10 * time.Minute))
			Expect(fakeClient.Create(ctx, other)).To(Succeed())

			action, err := handler.getAction(ctx, vd, log)
			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(revert))
		})

		It("should not follow the other disks when KubeVirt has not got the target of the disk", func() {
			vd := newDisk("disk-a", vdcondition.MigratingInProgressReason.String())
			vd.Status.MigrationState.TargetPVC = "disk-a-late-target"
			Expect(fakeClient.Create(ctx, newDisk("disk-b", vdcondition.MigratingWaitForSourceVolumeReleaseReason.String()))).To(Succeed())

			action, err := handler.getAction(ctx, vd, log)
			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(revert))
		})

		It("should find the VirtualMachine by the KVVM that has the target of the disk", func() {
			Expect(fakeClient.Create(ctx, &virtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "other-vm", Namespace: namespace},
				Spec:       virtv1.VirtualMachineSpec{Template: &virtv1.VirtualMachineInstanceTemplateSpec{}},
			})).To(Succeed())
			vd := newDisk("disk-a", vdcondition.MigratingInProgressReason.String())
			vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{{Name: "other-vm"}, {Name: vmName}}
			Expect(fakeClient.Create(ctx, newDisk("disk-b", vdcondition.MigratingWaitForSourceVolumeReleaseReason.String()))).To(Succeed())

			action, err := handler.getAction(ctx, vd, log)
			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(complete))
		})

		It("should not follow a disk that is not attached to the VirtualMachine", func() {
			vd := newDisk("disk-a", vdcondition.MigratingInProgressReason.String())
			Expect(fakeClient.Create(ctx, newDisk("disk-c", vdcondition.MigratingWaitForSourceVolumeReleaseReason.String()))).To(Succeed())

			action, err := handler.getAction(ctx, vd, log)
			Expect(err).NotTo(HaveOccurred())
			Expect(action).To(Equal(revert))
		})
	})

	Describe("handleComplete", func() {
		var vd *v1alpha2.VirtualDisk

		BeforeEach(func() {
			vd = newDisk("disk-a", vdcondition.MigratingInProgressReason.String())
			Expect(fakeClient.Create(ctx, newPVC(vd.Status.MigrationState.SourcePVC, vd))).To(Succeed())
			Expect(fakeClient.Create(ctx, newPVC(vd.Status.MigrationState.TargetPVC, vd))).To(Succeed())
		})

		It("should keep the source claim until the other disks wait for the source release", func() {
			Expect(fakeClient.Create(ctx, newDisk("disk-b", vdcondition.MigratingInProgressReason.String()))).To(Succeed())

			res, err := handler.handleComplete(ctx, vd)
			Expect(err).NotTo(HaveOccurred())
			Expect(res.RequeueAfter).To(Equal(finalizeMigrationRequeueAfter))
			Expect(pvcExists(vd.Status.MigrationState.SourcePVC)).To(BeTrue())

			cond, _ := conditions.GetCondition(vdcondition.MigratingType, vd.Status.Conditions)
			Expect(cond.Reason).To(Equal(vdcondition.MigratingWaitForSourceVolumeReleaseReason.String()))
			Expect(cond.Message).To(ContainSubstring("disk-b"))
		})

		It("should delete the source claim once the other disks wait for the source release", func() {
			Expect(fakeClient.Create(ctx, newDisk("disk-b", vdcondition.MigratingWaitForSourceVolumeReleaseReason.String()))).To(Succeed())

			_, err := handler.handleComplete(ctx, vd)
			Expect(err).NotTo(HaveOccurred())
			Expect(pvcExists(vd.Status.MigrationState.SourcePVC)).To(BeFalse())
			Expect(vd.Status.MigrationState.Result).To(Equal(v1alpha2.VirtualDiskMigrationResultSucceeded))
		})
	})

	Describe("isPreviousMigrationUnfinalized", func() {
		DescribeTable("should wait only for a disk that finalizes the previous migration",
			func(reason string, expected bool) {
				vd := newDisk("disk-a", "")
				vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{}
				Expect(fakeClient.Create(ctx, newDisk("disk-b", reason))).To(Succeed())

				vm := &v1alpha2.VirtualMachine{}
				Expect(fakeClient.Get(ctx, types.NamespacedName{Name: vmName, Namespace: namespace}, vm)).To(Succeed())
				unfinalized, err := handler.isPreviousMigrationUnfinalized(ctx, vd, vm, nil, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(unfinalized).To(Equal(expected))
			},
			Entry("waiting for the source release", vdcondition.MigratingWaitForSourceVolumeReleaseReason.String(), true),
			Entry("waiting for the target release", vdcondition.MigratingWaitForTargetVolumeReleaseReason.String(), true),
			Entry("preparing the current migration", vdcondition.MigratingWaitForTargetReadyReason.String(), false),
		)

		It("should wait for a disk that started migrating before the operation", func() {
			vd := newDisk("disk-a", "")
			vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{}
			Expect(fakeClient.Create(ctx, newDisk("disk-b", vdcondition.MigratingInProgressReason.String()))).To(Succeed())
			vmop := &v1alpha2.VirtualMachineOperation{
				ObjectMeta: metav1.ObjectMeta{Name: "migrate", Namespace: namespace, CreationTimestamp: metav1.NewTime(migrationStart.Add(time.Minute))},
				Spec:       v1alpha2.VirtualMachineOperationSpec{Type: v1alpha2.VMOPTypeMigrate, VirtualMachine: vmName},
				Status:     v1alpha2.VirtualMachineOperationStatus{Phase: v1alpha2.VMOPPhaseInProgress},
			}

			vm := &v1alpha2.VirtualMachine{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: vmName, Namespace: namespace}, vm)).To(Succeed())
			unfinalized, err := handler.isPreviousMigrationUnfinalized(ctx, vd, vm, vmop, log)
			Expect(err).NotTo(HaveOccurred())
			Expect(unfinalized).To(BeTrue())
		})
	})
})
