/*
Copyright 2025 Flant JSC

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
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
)

type fakeStorageClassValidator struct {
	allowedStorageClasses    map[string]bool
	deprecatedStorageClasses map[string]bool
}

func (m *fakeStorageClassValidator) IsStorageClassAllowed(scName string) bool {
	return m.allowedStorageClasses[scName]
}

func (m *fakeStorageClassValidator) IsStorageClassDeprecated(sc *storagev1.StorageClass) bool {
	return m.deprecatedStorageClasses[sc.Name]
}

type fakeVolumeAndAccessModesGetter struct {
	volumeMode  corev1.PersistentVolumeMode
	accessMode  corev1.PersistentVolumeAccessMode
	shouldError bool
}

func (m *fakeVolumeAndAccessModesGetter) GetVolumeAndAccessModes(_ context.Context, _ client.Object, _ *storagev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error) {
	if m.shouldError {
		return "", "", fmt.Errorf("mock error")
	}
	return m.volumeMode, m.accessMode, nil
}

var _ = Describe("MigrationHandler", func() {
	var (
		ctx              context.Context
		log              *slog.Logger
		scheme           *runtime.Scheme
		fakeClient       client.Client
		scValidator      *fakeStorageClassValidator
		modeGetter       *fakeVolumeAndAccessModesGetter
		migrationHandler *MigrationHandler
		vd               *v1alpha2.VirtualDisk
		vm               *v1alpha2.VirtualMachine
		kvvmi            *virtv1.VirtualMachineInstance
		storageClass     *storagev1.StorageClass
		pvc              *corev1.PersistentVolumeClaim
	)

	BeforeEach(func() {
		ctx = testutil.ContextBackgroundWithNoOpLogger()
		log = logger.FromContext(ctx)
		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		Expect(virtv1.AddToScheme(scheme)).To(Succeed())

		scValidator = &fakeStorageClassValidator{
			allowedStorageClasses: map[string]bool{
				"allowed-sc": true,
				"default-sc": true,
			},
			deprecatedStorageClasses: map[string]bool{
				"deprecated-sc": true,
			},
		}

		modeGetter = &fakeVolumeAndAccessModesGetter{
			volumeMode: corev1.PersistentVolumeBlock,
			accessMode: corev1.ReadWriteOnce,
		}

		// Create test VirtualDisk
		vd = &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vd",
				Namespace: "default",
				UID:       "test-uid",
			},
			Spec: v1alpha2.VirtualDiskSpec{
				PersistentVolumeClaim: v1alpha2.VirtualDiskPersistentVolumeClaim{
					StorageClass: ptr.To("allowed-sc"),
				},
			},
			Status: v1alpha2.VirtualDiskStatus{
				Capacity:         "10Gi",
				StorageClassName: "default-sc",
				Target: v1alpha2.DiskTarget{
					PersistentVolumeClaim: "test-pvc",
				},
			},
		}

		// Create test VirtualMachine
		vm = &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vm",
				Namespace: "default",
			},
			Status: v1alpha2.VirtualMachineStatus{
				Conditions: []metav1.Condition{},
			},
		}

		kvvmi = &virtv1.VirtualMachineInstance{
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualMachineInstance",
				APIVersion: "kubevirt.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vm",
				Namespace: "default",
			},
			Status: virtv1.VirtualMachineInstanceStatus{
				VolumeStatus: []virtv1.VolumeStatus{
					{
						Name: "vd-test-vd",
						Size: 10*104*1024*1024 + 2*1024*1024, // 10Gi + 2Mi overhead
					},
				},
			},
		}

		// Create test StorageClass
		storageClass = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "allowed-sc",
			},
			VolumeBindingMode: ptr.To(storagev1.VolumeBindingWaitForFirstConsumer),
		}

		// Create test PVC
		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						UID: "test-uid",
					},
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
			},
		}

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		migrationHandler = NewMigrationHandler(fakeClient, scValidator, modeGetter, featuregates.Default())
	})

	Describe("getAction", func() {
		Context("when disk is not in use", func() {
			BeforeEach(func() {
				vd.Status.Conditions = []metav1.Condition{
					{
						Type:   vdcondition.InUseType.String(),
						Status: metav1.ConditionFalse,
					},
				}
			})

			It("should return none", func() {
				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(none))
			})
		})

		Context("when no VM is currently mounted", func() {
			BeforeEach(func() {
				vd.Status.Conditions = []metav1.Condition{
					{
						Type:   vdcondition.InUseType.String(),
						Status: metav1.ConditionTrue,
						Reason: vdcondition.AttachedToVirtualMachine.String(),
					},
				}
			})

			It("should return none", func() {
				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(none))
			})
		})

		Context("when storage class has changed", func() {
			BeforeEach(func() {
				vd.Status.Conditions = []metav1.Condition{
					{
						Type:   vdcondition.InUseType.String(),
						Status: metav1.ConditionTrue,
						Reason: vdcondition.AttachedToVirtualMachine.String(),
					},
					{
						Type:   vdcondition.ReadyType.String(),
						Status: metav1.ConditionTrue,
						Reason: vdcondition.Ready.String(),
					},
				}
				vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{
					{
						Name:    "test-vm",
						Mounted: true,
					},
				}
				vd.Spec.PersistentVolumeClaim.StorageClass = ptr.To("allowed-sc")
				vd.Status.StorageClassName = "default-sc"

				vm.Status.Conditions = []metav1.Condition{
					{
						Type:   vmcondition.TypeMigrating.String(),
						Status: metav1.ConditionTrue,
						Reason: vmcondition.ReasonMigratingPending.String(),
					},
				}
				Expect(fakeClient.Create(ctx, vm)).To(Succeed())
			})

			It("should return migrate", func() {
				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(migratePrepareTarget))
			})

			It("should delay the new round while the KVVMI still records migrated volumes", func() {
				kvvmi.Status.MigratedVolumes = []virtv1.StorageMigratedVolumeInfo{
					{VolumeName: "vd-test-vd"},
				}
				Expect(fakeClient.Create(ctx, kvvmi)).To(Succeed())

				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(delayNewRound))
			})

			It("should delay the new round while the KVVMI volumes-change condition is set", func() {
				kvvmi.Status.Conditions = []virtv1.VirtualMachineInstanceCondition{
					{
						Type:   virtv1.VirtualMachineInstanceVolumesChange,
						Status: corev1.ConditionTrue,
					},
				}
				Expect(fakeClient.Create(ctx, kvvmi)).To(Succeed())

				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(delayNewRound))
			})

			It("should return migrate when the KVVMI carries no volume-change leftovers", func() {
				Expect(fakeClient.Create(ctx, kvvmi)).To(Succeed())

				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(migratePrepareTarget))
			})
		})

		Context("when the disk migration is in progress", func() {
			BeforeEach(func() {
				vd.Status.Conditions = []metav1.Condition{
					{
						Type:   vdcondition.InUseType.String(),
						Status: metav1.ConditionTrue,
						Reason: vdcondition.AttachedToVirtualMachine.String(),
					},
					{
						Type:   vdcondition.MigratingType.String(),
						Status: metav1.ConditionTrue,
						Reason: vdcondition.InProgress.String(),
					},
				}
				vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{
					{Name: "test-vm", Mounted: true},
				}
				vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
					SourcePVC:      "test-pvc",
					TargetPVC:      "target-pvc",
					StartTimestamp: metav1.Now(),
				}

				vm.Status.Conditions = []metav1.Condition{
					{
						Type:   vmcondition.TypeRunning.String(),
						Status: metav1.ConditionFalse,
						Reason: vmcondition.ReasonPodNotStarted.String(),
					},
					{
						Type:   vmcondition.TypeMigrating.String(),
						Status: metav1.ConditionTrue,
						Reason: vmcondition.ReasonMigratingInProgress.String(),
					},
				}
			})

			DescribeTable("should not revert while the phase keeps the VirtualMachine alive", func(phase v1alpha2.MachinePhase) {
				// The Running condition flaps for a moment at the switchover: for a
				// VirtualMachine with USB devices the hotplug attachment pod is recreated
				// on the migration target.
				vm.Status.Phase = phase
				Expect(fakeClient.Create(ctx, vm)).To(Succeed())

				vmop := &v1alpha2.VirtualMachineOperation{
					ObjectMeta: metav1.ObjectMeta{Name: "test-vmop", Namespace: "default"},
					Spec: v1alpha2.VirtualMachineOperationSpec{
						Type:           v1alpha2.VMOPTypeMigrate,
						VirtualMachine: "test-vm",
					},
					Status: v1alpha2.VirtualMachineOperationStatus{Phase: v1alpha2.VMOPPhaseInProgress},
				}
				Expect(fakeClient.Create(ctx, vmop)).To(Succeed())

				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(migrateSync))
			},
				Entry("running", v1alpha2.MachineRunning),
				Entry("migrating", v1alpha2.MachineMigrating),
			)

			It("should complete the migration once it succeeded despite the flapping condition", func() {
				vm.Status.Phase = v1alpha2.MachineRunning
				vm.Status.MigrationState = &v1alpha2.VirtualMachineMigrationState{
					StartTimestamp: ptr.To(metav1.NewTime(vd.Status.MigrationState.StartTimestamp.Add(time.Second))),
					EndTimestamp:   ptr.To(metav1.NewTime(vd.Status.MigrationState.StartTimestamp.Add(2 * time.Second))),
					Result:         v1alpha2.MigrationResultSucceeded,
				}
				Expect(fakeClient.Create(ctx, vm)).To(Succeed())

				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(complete))
			})

			It("should revert when the phase confirms the VirtualMachine is not running", func() {
				vm.Status.Phase = v1alpha2.MachineStopped
				Expect(fakeClient.Create(ctx, vm)).To(Succeed())

				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(revert))
			})

			// The window right after a successful switchover: the VM's Migrating
			// condition is already gone, no VMOP is in progress, and
			// vm.Status.MigrationState has not been updated for this round yet — but the
			// KVVMI already reports the migration succeeded and its volume points at the
			// target PVC. Reverting here would delete the PVC the guest runs on.
			It("should complete when the guest already switched to the target PVC", func() {
				vm.Status.Phase = v1alpha2.MachineRunning
				vm.Status.Conditions = nil
				Expect(fakeClient.Create(ctx, vm)).To(Succeed())

				kvvmi.Spec.Volumes = []virtv1.Volume{
					{
						Name: "vd-test-vd",
						VolumeSource: virtv1.VolumeSource{
							PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "target-pvc",
								},
							},
						},
					},
				}
				kvvmi.Status.MigrationState = &virtv1.VirtualMachineInstanceMigrationState{
					Completed:    true,
					EndTimestamp: ptr.To(metav1.NewTime(vd.Status.MigrationState.StartTimestamp.Add(2 * time.Second))),
				}
				Expect(fakeClient.Create(ctx, kvvmi)).To(Succeed())

				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(complete))
			})

			It("should still revert in that window when the guest stayed on the source PVC", func() {
				vm.Status.Phase = v1alpha2.MachineRunning
				vm.Status.Conditions = nil
				Expect(fakeClient.Create(ctx, vm)).To(Succeed())

				kvvmi.Spec.Volumes = []virtv1.Volume{
					{
						Name: "vd-test-vd",
						VolumeSource: virtv1.VolumeSource{
							PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "test-pvc",
								},
							},
						},
					},
				}
				kvvmi.Status.MigrationState = &virtv1.VirtualMachineInstanceMigrationState{
					Completed:    true,
					EndTimestamp: ptr.To(metav1.NewTime(vd.Status.MigrationState.StartTimestamp.Add(2 * time.Second))),
				}
				Expect(fakeClient.Create(ctx, kvvmi)).To(Succeed())

				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(revert))
			})

			It("should not complete off a migration that ended before this round started", func() {
				vm.Status.Phase = v1alpha2.MachineRunning
				vm.Status.Conditions = nil
				Expect(fakeClient.Create(ctx, vm)).To(Succeed())

				kvvmi.Spec.Volumes = []virtv1.Volume{
					{
						Name: "vd-test-vd",
						VolumeSource: virtv1.VolumeSource{
							PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "target-pvc",
								},
							},
						},
					},
				}
				kvvmi.Status.MigrationState = &virtv1.VirtualMachineInstanceMigrationState{
					Completed:    true,
					EndTimestamp: ptr.To(metav1.NewTime(vd.Status.MigrationState.StartTimestamp.Add(-time.Minute))),
				}
				Expect(fakeClient.Create(ctx, kvvmi)).To(Succeed())

				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(revert))
			})
		})

		Context("when disks should be migrating", func() {
			createMigratingVMOP := func(completedReason vmopcondition.ReasonCompleted) {
				vmop := &v1alpha2.VirtualMachineOperation{
					ObjectMeta: metav1.ObjectMeta{Name: "test-vmop", Namespace: "default"},
					Spec: v1alpha2.VirtualMachineOperationSpec{
						Type:           v1alpha2.VMOPTypeMigrate,
						VirtualMachine: "test-vm",
					},
					Status: v1alpha2.VirtualMachineOperationStatus{
						Phase: v1alpha2.VMOPPhaseInProgress,
						Conditions: []metav1.Condition{
							{
								Type:   vmopcondition.TypeCompleted.String(),
								Status: metav1.ConditionFalse,
								Reason: completedReason.String(),
							},
						},
					},
				}
				Expect(fakeClient.Create(ctx, vmop)).To(Succeed())
			}

			BeforeEach(func() {
				vd.Status.Conditions = []metav1.Condition{
					{
						Type:   vdcondition.InUseType.String(),
						Status: metav1.ConditionTrue,
						Reason: vdcondition.AttachedToVirtualMachine.String(),
					},
					{
						Type:   vdcondition.ReadyType.String(),
						Status: metav1.ConditionTrue,
						Reason: vdcondition.Ready.String(),
					},
				}
				vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{
					{Name: "test-vm", Mounted: true},
				}
				// Keep the storage class unchanged so getAction takes the
				// disks-should-be-migrating branch, not the storage-class-changed one.
				vd.Spec.PersistentVolumeClaim.StorageClass = ptr.To("default-sc")
				vd.Status.StorageClassName = "default-sc"

				vm.Status.Conditions = []metav1.Condition{
					{
						Type:   vmcondition.TypeMigrating.String(),
						Status: metav1.ConditionTrue,
						Reason: vmcondition.ReasonMigratingPending.String(),
					},
					{
						Type:   vmcondition.TypeMigratable.String(),
						Status: metav1.ConditionTrue,
						Reason: vmcondition.ReasonDisksShouldBeMigrating.String(),
					},
				}
				Expect(fakeClient.Create(ctx, vm)).To(Succeed())
				Expect(fakeClient.Create(ctx, pvc)).To(Succeed())
			})

			It("should prepare the target when the operation is waiting for disks", func() {
				createMigratingVMOP(vmopcondition.ReasonWaitingForVirtualMachineToBeReadyToMigrate)

				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(migratePrepareTarget))
			})

			It("should delay the new round while the KVVMI still records migrated volumes", func() {
				createMigratingVMOP(vmopcondition.ReasonWaitingForVirtualMachineToBeReadyToMigrate)

				kvvmi.Status.MigratedVolumes = []virtv1.StorageMigratedVolumeInfo{
					{VolumeName: "vd-test-vd"},
				}
				Expect(fakeClient.Create(ctx, kvvmi)).To(Succeed())

				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(delayNewRound))
			})

			It("should not start a new migration once the operation is past waiting for disks", func() {
				// The compute migration has already started or is finalizing: starting a
				// new disk migration here would overwrite the migration state and cause a
				// revert loop.
				createMigratingVMOP(vmopcondition.ReasonMigrationRunning)

				action, err := migrationHandler.getAction(ctx, vd, log)
				Expect(err).NotTo(HaveOccurred())
				Expect(action).To(Equal(none))
			})

			// The reason of the Migratable condition answers two questions at once: whether
			// the disks travel along with the machine, and whether the cluster has a node to
			// take it. Once there is no target, the answer of the target check is reported
			// instead, so the disks-should-be-migrating reason is gone and the target volumes
			// are not prepared for a migration that cannot start anyway. That holds for either
			// answer of the target check, and both of them must keep the preparation off: the
			// nodes matching the placement rules may be missing altogether, or they may be
			// unable to take the machine at the moment.
			DescribeTable("should not prepare the target while the cluster has no node to migrate to",
				func(status metav1.ConditionStatus, reason vmcondition.MigratableReason) {
					createMigratingVMOP(vmopcondition.ReasonWaitingForVirtualMachineToBeReadyToMigrate)

					vm.Status.Conditions = []metav1.Condition{
						{
							Type:   vmcondition.TypeMigrating.String(),
							Status: metav1.ConditionTrue,
							Reason: vmcondition.ReasonMigratingPending.String(),
						},
						{
							Type:   vmcondition.TypeMigratable.String(),
							Status: status,
							Reason: reason.String(),
						},
					}
					Expect(fakeClient.Update(ctx, vm)).To(Succeed())

					action, err := migrationHandler.getAction(ctx, vd, log)
					Expect(err).NotTo(HaveOccurred())
					Expect(action).To(Equal(none))
				},
				Entry("no node of the cluster matches the placement rules",
					metav1.ConditionFalse, vmcondition.ReasonNoMigrationTarget),
				Entry("the matching nodes cannot take the machine at the moment",
					metav1.ConditionTrue, vmcondition.ReasonWaitingForMigrationTarget),
			)
		})
	})

	Describe("handleMigrate", func() {
		Context("when disk is being resized", func() {
			BeforeEach(func() {
				vd.Status.Conditions = []metav1.Condition{
					{
						Type:   vdcondition.ResizingType.String(),
						Status: metav1.ConditionTrue,
					},
				}
			})

			It("should set pending condition", func() {
				err := migrationHandler.handleMigratePrepareTarget(ctx, vd)
				Expect(err).NotTo(HaveOccurred())

				migrating, found := conditions.GetCondition(vdcondition.MigratingType, vd.Status.Conditions)
				Expect(found).To(BeTrue())
				Expect(migrating.Status).To(Equal(metav1.ConditionFalse))
				Expect(migrating.Reason).To(Equal(vdcondition.ResizingInProgressReason.String()))
			})
		})

		Context("when storage class is not allowed", func() {
			BeforeEach(func() {
				vd.Spec.PersistentVolumeClaim.StorageClass = ptr.To("not-allowed-sc")
				storageClass.Name = "not-allowed-sc"
				Expect(fakeClient.Create(ctx, storageClass)).To(Succeed())
			})

			It("should set failed migration state", func() {
				err := migrationHandler.handleMigratePrepareTarget(ctx, vd)
				Expect(err).NotTo(HaveOccurred())

				Expect(vd.Status.MigrationState.Result).To(Equal(v1alpha2.VirtualDiskMigrationResultFailed))
				Expect(vd.Status.MigrationState.Message).To(ContainSubstring("not allowed"))
			})
		})

		Context("when storage class is deprecated", func() {
			BeforeEach(func() {
				vd.Spec.PersistentVolumeClaim.StorageClass = ptr.To("deprecated-sc")
				storageClass.Name = "deprecated-sc"
				Expect(fakeClient.Create(ctx, storageClass)).To(Succeed())
			})

			It("should set failed migration state", func() {
				err := migrationHandler.handleMigratePrepareTarget(ctx, vd)
				Expect(err).NotTo(HaveOccurred())

				Expect(vd.Status.MigrationState.Result).To(Equal(v1alpha2.VirtualDiskMigrationResultFailed))
				Expect(vd.Status.MigrationState.Message).To(ContainSubstring("deprecated"))
			})
		})

		Context("when migration is successful", func() {
			BeforeEach(func() {
				Expect(fakeClient.Create(ctx, storageClass)).To(Succeed())
				pvc.Status.Capacity = corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				}
				vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{
					{
						Name:    "test-vm",
						Mounted: true,
					},
				}
				Expect(fakeClient.Create(ctx, pvc)).To(Succeed())
				Expect(fakeClient.Create(ctx, kvvmi)).To(Succeed())
			})

			It("should start migration", func() {
				err := migrationHandler.handleMigratePrepareTarget(ctx, vd)
				Expect(err).NotTo(HaveOccurred())

				Expect(vd.Status.MigrationState.StartTimestamp).NotTo(BeZero())
				Expect(vd.Status.MigrationState.SourcePVC).To(Equal("test-pvc"))
				Expect(vd.Status.MigrationState.TargetPVC).NotTo(BeEmpty())

				// The condition will be False because handleMigrateSync is called immediately
				// and the target PVC doesn't exist in the fake client yet
				migrating, found := conditions.GetCondition(vdcondition.MigratingType, vd.Status.Conditions)
				Expect(found).To(BeTrue())
				Expect(migrating.Status).To(Equal(metav1.ConditionFalse))
				Expect(migrating.Reason).To(Equal(vdcondition.MigratingWaitForTargetReadyReason.String()))
			})
		})
	})

	Describe("createTargetPersistentVolumeClaim", func() {
		var size resource.Quantity

		BeforeEach(func() {
			size = resource.MustParse("10Gi")
		})

		It("should ignore stale target PVC name when it matches source PVC name", func() {
			sourcePVC := newEmptyPVC("source-pvc", "default")
			withOwner(sourcePVC, vd)
			Expect(fakeClient.Create(ctx, sourcePVC)).To(Succeed())

			targetPVC := newEmptyPVC("target-pvc", "default")
			withOwner(targetPVC, vd)
			Expect(fakeClient.Create(ctx, targetPVC)).To(Succeed())

			pvc, err := migrationHandler.createTargetPersistentVolumeClaim(ctx, vd, storageClass, size, "source-pvc", "source-pvc", corev1.PersistentVolumeBlock, corev1.ReadWriteOnce)
			Expect(err).NotTo(HaveOccurred())
			Expect(pvc.Name).To(Equal("target-pvc"))
		})

		It("should select the only non-source PVC when target PVC name is empty", func() {
			sourcePVC := newEmptyPVC("source-pvc", "default")
			withOwner(sourcePVC, vd)
			Expect(fakeClient.Create(ctx, sourcePVC)).To(Succeed())

			targetPVC := newEmptyPVC("target-pvc", "default")
			withOwner(targetPVC, vd)
			Expect(fakeClient.Create(ctx, targetPVC)).To(Succeed())

			pvc, err := migrationHandler.createTargetPersistentVolumeClaim(ctx, vd, storageClass, size, "", "source-pvc", corev1.PersistentVolumeBlock, corev1.ReadWriteOnce)
			Expect(err).NotTo(HaveOccurred())
			Expect(pvc.Name).To(Equal("target-pvc"))
		})

		It("should select the only non-source PVC when target PVC name is specified but not found", func() {
			sourcePVC := newEmptyPVC("source-pvc", "default")
			withOwner(sourcePVC, vd)
			Expect(fakeClient.Create(ctx, sourcePVC)).To(Succeed())

			otherPVC := newEmptyPVC("other-pvc", "default")
			withOwner(otherPVC, vd)
			Expect(fakeClient.Create(ctx, otherPVC)).To(Succeed())

			pvc, err := migrationHandler.createTargetPersistentVolumeClaim(ctx, vd, storageClass, size, "missing-pvc", "source-pvc", corev1.PersistentVolumeBlock, corev1.ReadWriteOnce)
			Expect(err).NotTo(HaveOccurred())
			Expect(pvc.Name).To(Equal("other-pvc"))
		})

		It("should skip when source PVC was not found", func() {
			ownedPVC := newEmptyPVC("owned-pvc", "default")
			withOwner(ownedPVC, vd)
			Expect(fakeClient.Create(ctx, ownedPVC)).To(Succeed())

			pvc, err := migrationHandler.createTargetPersistentVolumeClaim(ctx, vd, storageClass, size, "", "missing-source-pvc", corev1.PersistentVolumeBlock, corev1.ReadWriteOnce)
			Expect(err).NotTo(HaveOccurred())
			Expect(pvc).To(BeNil())
		})

		It("should select target PVC by name when it is specified and found", func() {
			sourcePVC := newEmptyPVC("source-pvc", "default")
			withOwner(sourcePVC, vd)
			Expect(fakeClient.Create(ctx, sourcePVC)).To(Succeed())

			targetPVC := newEmptyPVC("target-pvc", "default")
			withOwner(targetPVC, vd)
			Expect(fakeClient.Create(ctx, targetPVC)).To(Succeed())

			pvc, err := migrationHandler.createTargetPersistentVolumeClaim(ctx, vd, storageClass, size, "target-pvc", "source-pvc", corev1.PersistentVolumeBlock, corev1.ReadWriteOnce)
			Expect(err).NotTo(HaveOccurred())
			Expect(pvc.Name).To(Equal("target-pvc"))
		})

		It("should skip when target PVC cannot be selected unambiguously", func() {
			sourcePVC := newEmptyPVC("source-pvc", "default")
			withOwner(sourcePVC, vd)
			Expect(fakeClient.Create(ctx, sourcePVC)).To(Succeed())

			firstPVC := newEmptyPVC("first-pvc", "default")
			withOwner(firstPVC, vd)
			Expect(fakeClient.Create(ctx, firstPVC)).To(Succeed())

			secondPVC := newEmptyPVC("second-pvc", "default")
			withOwner(secondPVC, vd)
			Expect(fakeClient.Create(ctx, secondPVC)).To(Succeed())

			pvc, err := migrationHandler.createTargetPersistentVolumeClaim(ctx, vd, storageClass, size, "", "source-pvc", corev1.PersistentVolumeBlock, corev1.ReadWriteOnce)
			Expect(err).NotTo(HaveOccurred())
			Expect(pvc).To(BeNil())
		})
	})

	Describe("handleRevert", func() {
		BeforeEach(func() {
			vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
				SourcePVC: "source-pvc",
				TargetPVC: "target-pvc",
			}
		})

		Context("when target PVC exists", func() {
			BeforeEach(func() {
				sourcePVC := newEmptyPVC("source-pvc", "default")
				withOwner(sourcePVC, vd)
				Expect(fakeClient.Create(ctx, sourcePVC)).To(Succeed())

				targetPVC := newEmptyPVC("target-pvc", "default")
				withOwner(targetPVC, vd)
				Expect(fakeClient.Create(ctx, targetPVC)).To(Succeed())
			})

			It("should delete target PVC and set failed state", func() {
				_, err := migrationHandler.handleRevert(ctx, vd)
				Expect(err).NotTo(HaveOccurred())

				Expect(vd.Status.MigrationState.EndTimestamp).NotTo(BeZero())
				Expect(vd.Status.MigrationState.Result).To(Equal(v1alpha2.VirtualDiskMigrationResultFailed))
				Expect(vd.Status.MigrationState.Message).To(Equal("Migration reverted."))

				// Check that migrating condition is removed
				_, found := conditions.GetCondition(vdcondition.MigratingType, vd.Status.Conditions)
				Expect(found).To(BeFalse())
			})
		})

		Context("when target PVC does not exist", func() {
			It("should set failed state without error", func() {
				_, err := migrationHandler.handleRevert(ctx, vd)
				Expect(err).NotTo(HaveOccurred())

				Expect(vd.Status.MigrationState.EndTimestamp).NotTo(BeZero())
				Expect(vd.Status.MigrationState.Result).To(Equal(v1alpha2.VirtualDiskMigrationResultFailed))
				Expect(vd.Status.MigrationState.Message).To(Equal("Migration reverted."))
			})
		})
	})

	Describe("handleComplete", func() {
		BeforeEach(func() {
			vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
				SourcePVC: "source-pvc",
				TargetPVC: "target-pvc",
			}
		})

		Context("when target PVC is not found", func() {
			It("should set failed state and revert to source PVC", func() {
				_, err := migrationHandler.handleComplete(ctx, vd)
				Expect(err).NotTo(HaveOccurred())

				Expect(vd.Status.MigrationState.EndTimestamp).NotTo(BeZero())
				Expect(vd.Status.MigrationState.Result).To(Equal(v1alpha2.VirtualDiskMigrationResultFailed))
				Expect(vd.Status.MigrationState.Message).To(ContainSubstring("target PVC is not found"))

				// Check that migrating condition is removed
				_, found := conditions.GetCondition(vdcondition.MigratingType, vd.Status.Conditions)
				Expect(found).To(BeFalse())
			})
		})

		Context("when target PVC is not bound", func() {
			BeforeEach(func() {
				targetPVC := newEmptyPVC("target-pvc", "default")
				withOwner(targetPVC, vd)
				targetPVC.Status = corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimPending,
				}
				Expect(fakeClient.Create(ctx, targetPVC)).To(Succeed())
			})

			It("should delete target PVC and set failed state", func() {
				_, err := migrationHandler.handleComplete(ctx, vd)
				Expect(err).NotTo(HaveOccurred())

				Expect(vd.Status.MigrationState.EndTimestamp).NotTo(BeZero())
				Expect(vd.Status.MigrationState.Result).To(Equal(v1alpha2.VirtualDiskMigrationResultFailed))
				Expect(vd.Status.MigrationState.Message).To(ContainSubstring("target PVC is not bound"))
			})
		})

		Context("when migration is successful", func() {
			BeforeEach(func() {
				sourcePVC := newEmptyPVC("source-pvc", "default")
				withOwner(sourcePVC, vd)
				Expect(fakeClient.Create(ctx, sourcePVC)).To(Succeed())

				targetPVC := newEmptyPVC("target-pvc", "default")
				targetPVC.Status = corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimBound,
				}
				withOwner(targetPVC, vd)
				Expect(fakeClient.Create(ctx, targetPVC)).To(Succeed())
			})

			It("should complete migration successfully", func() {
				_, err := migrationHandler.handleComplete(ctx, vd)
				Expect(err).NotTo(HaveOccurred())

				Expect(vd.Status.MigrationState.EndTimestamp).NotTo(BeZero())
				Expect(vd.Status.MigrationState.Result).To(Equal(v1alpha2.VirtualDiskMigrationResultSucceeded))
				Expect(vd.Status.MigrationState.Message).To(Equal("Migration completed."))

				// Check that migrating condition is removed
				_, found := conditions.GetCondition(vdcondition.MigratingType, vd.Status.Conditions)
				Expect(found).To(BeFalse())
			})
		})
	})

	Describe("canDeletePersistentVolumeClaim", func() {
		BeforeEach(func() {
			vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
				SourcePVC: "source-pvc",
				TargetPVC: "target-pvc",
			}

			targetPVC := newEmptyPVC("target-pvc", "default")
			withOwner(targetPVC, vd)
			targetPVC.Spec.VolumeName = "pv-target"
			targetPVC.Status.Phase = corev1.ClaimBound
			Expect(fakeClient.Create(ctx, targetPVC)).To(Succeed())
		})

		Context("when the disk is not mounted to any VirtualMachine", func() {
			It("should allow the deletion", func() {
				canDelete, reason, err := migrationHandler.canDeletePersistentVolumeClaim(ctx, vd, "target-pvc", targetPVCRole)
				Expect(err).NotTo(HaveOccurred())
				Expect(canDelete).To(BeTrue())
				Expect(reason).To(BeEmpty())
			})

			It("should not look at the pods and the internal resources", func() {
				Expect(fakeClient.Create(ctx, newVirtLauncherPod("virt-launcher-test-vm-abcde", "test-vm", corev1.PodRunning, "target-pvc"))).To(Succeed())
				Expect(fakeClient.Create(ctx, newKVVMWithPVC("test-vm", "target-pvc"))).To(Succeed())

				canDelete, _, err := migrationHandler.canDeletePersistentVolumeClaim(ctx, vd, "target-pvc", targetPVCRole)
				Expect(err).NotTo(HaveOccurred())
				Expect(canDelete).To(BeTrue())
			})

			It("should allow the deletion of the claim that is not found", func() {
				canDelete, _, err := migrationHandler.canDeletePersistentVolumeClaim(ctx, vd, "unknown-pvc", targetPVCRole)
				Expect(err).NotTo(HaveOccurred())
				Expect(canDelete).To(BeTrue())
			})
		})

		Context("when the disk is mounted to a VirtualMachine", func() {
			BeforeEach(func() {
				vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{
					{Name: "test-vm", Mounted: true},
				}
				Expect(fakeClient.Create(ctx, vm)).To(Succeed())
			})

			It("should allow the deletion when nothing uses the claim", func() {
				canDelete, reason, err := migrationHandler.canDeletePersistentVolumeClaim(ctx, vd, "target-pvc", targetPVCRole)
				Expect(err).NotTo(HaveOccurred())
				Expect(canDelete).To(BeTrue())
				Expect(reason).To(BeEmpty())
			})

			It("should forbid the deletion when the internal VirtualMachine references the claim", func() {
				Expect(fakeClient.Create(ctx, newKVVMWithPVC("test-vm", "target-pvc"))).To(Succeed())

				canDelete, reason, err := migrationHandler.canDeletePersistentVolumeClaim(ctx, vd, "target-pvc", targetPVCRole)
				Expect(err).NotTo(HaveOccurred())
				Expect(canDelete).To(BeFalse())
				Expect(reason).To(Equal(`The target PersistentVolumeClaim "target-pvc" is still in use by the VirtualMachine "test-vm".`))
			})

			It("should forbid the deletion when the internal VirtualMachineInstance references the claim", func() {
				kvvmi.Spec.Volumes = []virtv1.Volume{
					{
						Name: "vd-test-vd",
						VolumeSource: virtv1.VolumeSource{
							PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "target-pvc"},
							},
						},
					},
				}
				Expect(fakeClient.Create(ctx, kvvmi)).To(Succeed())

				canDelete, reason, err := migrationHandler.canDeletePersistentVolumeClaim(ctx, vd, "target-pvc", targetPVCRole)
				Expect(err).NotTo(HaveOccurred())
				Expect(canDelete).To(BeFalse())
				Expect(reason).To(Equal(`The target PersistentVolumeClaim "target-pvc" is still in use by the VirtualMachine "test-vm".`))
			})

			DescribeTable("should account for the pods of the VirtualMachine",
				func(phase corev1.PodPhase, expectedCanDelete bool) {
					Expect(fakeClient.Create(ctx, newVirtLauncherPod("virt-launcher-test-vm-abcde", "test-vm", phase, "target-pvc"))).To(Succeed())

					canDelete, reason, err := migrationHandler.canDeletePersistentVolumeClaim(ctx, vd, "target-pvc", targetPVCRole)
					Expect(err).NotTo(HaveOccurred())
					Expect(canDelete).To(Equal(expectedCanDelete))
					if !expectedCanDelete {
						Expect(reason).To(Equal(`The target PersistentVolumeClaim "target-pvc" is still in use by the running VirtualMachine "test-vm".`))
					}
				},
				Entry("running pod holds the claim", corev1.PodRunning, false),
				Entry("pending pod holds the claim", corev1.PodPending, false),
				Entry("succeeded pod is ignored", corev1.PodSucceeded, true),
				Entry("failed pod is ignored", corev1.PodFailed, true),
			)

			It("should ignore the pods of the other VirtualMachine", func() {
				Expect(fakeClient.Create(ctx, newVirtLauncherPod("virt-launcher-other-vm-abcde", "other-vm", corev1.PodRunning, "target-pvc"))).To(Succeed())

				canDelete, _, err := migrationHandler.canDeletePersistentVolumeClaim(ctx, vd, "target-pvc", targetPVCRole)
				Expect(err).NotTo(HaveOccurred())
				Expect(canDelete).To(BeTrue())
			})

			It("should ignore the pods that do not use the claim", func() {
				Expect(fakeClient.Create(ctx, newVirtLauncherPod("virt-launcher-test-vm-abcde", "test-vm", corev1.PodRunning, "source-pvc"))).To(Succeed())

				canDelete, _, err := migrationHandler.canDeletePersistentVolumeClaim(ctx, vd, "target-pvc", targetPVCRole)
				Expect(err).NotTo(HaveOccurred())
				Expect(canDelete).To(BeTrue())
			})
		})

		Context("when it is called for the source and the target claims", func() {
			BeforeEach(func() {
				vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{
					{Name: "test-vm", Mounted: true},
				}
				Expect(fakeClient.Create(ctx, vm)).To(Succeed())

				sourcePVC := newEmptyPVC("source-pvc", "default")
				withOwner(sourcePVC, vd)
				sourcePVC.Spec.VolumeName = "pv-source"
				sourcePVC.Status.Phase = corev1.ClaimBound
				Expect(fakeClient.Create(ctx, sourcePVC)).To(Succeed())

				kvvm := newKVVMWithPVC("test-vm", "target-pvc")
				kvvm.Spec.Template.Spec.Volumes = append(kvvm.Spec.Template.Spec.Volumes, virtv1.Volume{
					Name: "vd-test-vd-source",
					VolumeSource: virtv1.VolumeSource{
						PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: "source-pvc"},
						},
					},
				})
				Expect(fakeClient.Create(ctx, kvvm)).To(Succeed())
			})

			It("should name the target claim on revert", func() {
				canFinalize, reason, err := migrationHandler.canFinalizeRevert(ctx, vd)
				Expect(err).NotTo(HaveOccurred())
				Expect(canFinalize).To(BeFalse())
				Expect(reason).To(Equal(`The target PersistentVolumeClaim "target-pvc" is still in use by the VirtualMachine "test-vm".`))
			})

			It("should name the source claim on complete", func() {
				canFinalize, reason, err := migrationHandler.canFinalizeComplete(ctx, vd)
				Expect(err).NotTo(HaveOccurred())
				Expect(canFinalize).To(BeFalse())
				Expect(reason).To(Equal(`The source PersistentVolumeClaim "source-pvc" is still in use by the VirtualMachine "test-vm".`))
			})
		})
	})

	Describe("finalization is blocked by a claim in use", func() {
		BeforeEach(func() {
			vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
				SourcePVC: "source-pvc",
				TargetPVC: "target-pvc",
			}
			vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{
				{Name: "test-vm", Mounted: true},
			}
			Expect(fakeClient.Create(ctx, vm)).To(Succeed())

			sourcePVC := newEmptyPVC("source-pvc", "default")
			withOwner(sourcePVC, vd)
			sourcePVC.Status.Phase = corev1.ClaimBound
			Expect(fakeClient.Create(ctx, sourcePVC)).To(Succeed())

			targetPVC := newEmptyPVC("target-pvc", "default")
			withOwner(targetPVC, vd)
			targetPVC.Status.Phase = corev1.ClaimBound
			Expect(fakeClient.Create(ctx, targetPVC)).To(Succeed())
		})

		It("should keep the target claim and requeue on revert", func() {
			Expect(fakeClient.Create(ctx, newVirtLauncherPod("virt-launcher-test-vm-abcde", "test-vm", corev1.PodRunning, "target-pvc"))).To(Succeed())

			result, err := migrationHandler.handleRevert(ctx, vd)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(finalizeMigrationRequeueAfter))

			Expect(vd.Status.MigrationState.EndTimestamp).To(BeZero())
			Expect(vd.Status.MigrationState.Result).To(BeEmpty())

			cond, found := conditions.GetCondition(vdcondition.MigratingType, vd.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vdcondition.MigratingWaitForTargetVolumeReleaseReason.String()))
			Expect(cond.Message).To(Equal(`Cannot revert the migration. The target PersistentVolumeClaim "target-pvc" is still in use by the running VirtualMachine "test-vm".`))

			pvc := &corev1.PersistentVolumeClaim{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "target-pvc", Namespace: "default"}, pvc)).To(Succeed())
			Expect(pvc.DeletionTimestamp).To(BeNil())
		})

		It("should keep the source claim and requeue on complete", func() {
			Expect(fakeClient.Create(ctx, newVirtLauncherPod("virt-launcher-test-vm-abcde", "test-vm", corev1.PodRunning, "source-pvc"))).To(Succeed())

			result, err := migrationHandler.handleComplete(ctx, vd)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(finalizeMigrationRequeueAfter))

			Expect(vd.Status.MigrationState.EndTimestamp).To(BeZero())
			Expect(vd.Status.MigrationState.Result).To(BeEmpty())

			cond, found := conditions.GetCondition(vdcondition.MigratingType, vd.Status.Conditions)
			Expect(found).To(BeTrue())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(vdcondition.MigratingWaitForSourceVolumeReleaseReason.String()))
			Expect(cond.Message).To(Equal(`Cannot complete the migration. The source PersistentVolumeClaim "source-pvc" is still in use by the running VirtualMachine "test-vm".`))

			pvc := &corev1.PersistentVolumeClaim{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "source-pvc", Namespace: "default"}, pvc)).To(Succeed())
			Expect(pvc.DeletionTimestamp).To(BeNil())
		})
	})
})

//nolint:unparam // test helper
func newEmptyPVC(name, namespace string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolumeClaim",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func withOwner(pvc *corev1.PersistentVolumeClaim, owner client.Object) {
	pvc.OwnerReferences = []metav1.OwnerReference{service.MakeControllerOwnerReference(owner)}
}

func newVirtLauncherPod(name, vmName string, phase corev1.PodPhase, claimNames ...string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels: map[string]string{
				virtv1.VirtualMachineNameLabel: vmName,
			},
		},
		Status: corev1.PodStatus{
			Phase: phase,
		},
	}

	for i, claimName := range claimNames {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: fmt.Sprintf("volume-%d", i),
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claimName},
			},
		})
	}

	return pod
}

func newKVVMWithPVC(name, claimName string) *virtv1.VirtualMachine {
	return &virtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: virtv1.VirtualMachineSpec{
			Template: &virtv1.VirtualMachineInstanceTemplateSpec{
				Spec: virtv1.VirtualMachineInstanceSpec{
					Volumes: []virtv1.Volume{
						{
							Name: "vd-test-vd",
							VolumeSource: virtv1.VolumeSource{
								PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: claimName},
								},
							},
						},
					},
				},
			},
		},
	}
}
