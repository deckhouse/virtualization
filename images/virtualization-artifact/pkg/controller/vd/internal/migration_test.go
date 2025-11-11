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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
				err := migrationHandler.handleRevert(ctx, vd)
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
				err := migrationHandler.handleRevert(ctx, vd)
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
				err := migrationHandler.handleComplete(ctx, vd)
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
				err := migrationHandler.handleComplete(ctx, vd)
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
				err := migrationHandler.handleComplete(ctx, vd)
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
	pvc.ObjectMeta.OwnerReferences = []metav1.OwnerReference{service.MakeControllerOwnerReference(owner)}
}
