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

package service

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("MigrationVolumesService", func() {
	const (
		vmName    = "vm-test"
		namespace = "default"
		sourcePVC = "disk-source"
		targetPVC = "disk-target"
	)

	newVM := func() *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualMachineKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:       vmName,
				Namespace:  namespace,
				Generation: 1,
			},
			Spec: v1alpha2.VirtualMachineSpec{},
		}
	}

	newKVVMWithVolume := func(pvcName string, strategy *virtv1.UpdateVolumesStrategy, nodeLabel string) *virtv1.VirtualMachine {
		return &virtv1.VirtualMachine{
			TypeMeta: metav1.TypeMeta{
				APIVersion: virtv1.GroupVersion.String(),
				Kind:       "VirtualMachine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: namespace,
			},
			Spec: virtv1.VirtualMachineSpec{
				UpdateVolumesStrategy: strategy,
				Template: &virtv1.VirtualMachineInstanceTemplateSpec{
					Spec: virtv1.VirtualMachineInstanceSpec{
						Affinity: &corev1.Affinity{
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "kubernetes.io/hostname",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{nodeLabel},
												},
											},
										},
									},
								},
							},
						},
						Volumes: []virtv1.Volume{
							{
								Name: "rootdisk",
								VolumeSource: virtv1.VolumeSource{
									PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: pvcName,
										},
									},
								},
							},
						},
					},
				},
			},
		}
	}

	setupState := func(vm *v1alpha2.VirtualMachine, objs ...client.Object) state.VirtualMachineState {
		allObjects := append([]client.Object{vm}, objs...)
		fakeClient, err := testutil.NewFakeClientWithObjects(allObjects...)
		Expect(err).NotTo(HaveOccurred())

		resource := reconciler.NewResource(client.ObjectKeyFromObject(vm), fakeClient,
			func() *v1alpha2.VirtualMachine {
				return &v1alpha2.VirtualMachine{}
			},
			func(obj *v1alpha2.VirtualMachine) v1alpha2.VirtualMachineStatus {
				return obj.Status
			},
		)
		Expect(resource.Fetch(context.Background())).To(Succeed())

		return state.New(fakeClient, resource)
	}

	newKVVMIWithVolume := func(pvcName string) *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			TypeMeta: metav1.TypeMeta{
				APIVersion: virtv1.GroupVersion.String(),
				Kind:       "VirtualMachineInstance",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: namespace,
			},
			Spec: virtv1.VirtualMachineInstanceSpec{
				Volumes: []virtv1.Volume{
					{
						Name: "rootdisk",
						VolumeSource: virtv1.VolumeSource{
							PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvcName,
								},
							},
						},
					},
				},
			},
		}
	}

	appendVolume := func(kvvm *virtv1.VirtualMachine, name, pvcName string) *virtv1.VirtualMachine {
		kvvm.Spec.Template.Spec.Volumes = append(kvvm.Spec.Template.Spec.Volumes, virtv1.Volume{
			Name: name,
			VolumeSource: virtv1.VolumeSource{
				PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvcName,
					},
				},
			},
		})
		return kvvm
	}

	It("does not apply structural volume changes to kvvm while restart is required", func() {
		ctx := testutil.ContextBackgroundWithNoOpLogger()

		vm := newVM()
		kvvmInCluster := newKVVMWithVolume(sourcePVC, nil, "source-node")
		kvvmi := newKVVMIWithVolume(sourcePVC)
		// The desired spec adds a second disk: a structural change that may require
		// a restart and must not be propagated to KVVM while the VM awaits restart.
		desiredKVVM := appendVolume(newKVVMWithVolume(sourcePVC, nil, "source-node"), "extradisk", "disk-extra")
		vmState := setupState(vm, kvvmInCluster, kvvmi)

		service := NewMigrationVolumesService(
			vmState.Client(),
			func(context.Context, state.VirtualMachineState) (*virtv1.VirtualMachine, error) {
				return desiredKVVM.DeepCopy(), nil
			},
			10*time.Second,
		)

		_, err := service.SyncVolumes(ctx, vmState, true)
		Expect(err).NotTo(HaveOccurred())

		updatedKVVM := &virtv1.VirtualMachine{}
		Expect(vmState.Client().Get(ctx, types.NamespacedName{Name: vmName, Namespace: namespace}, updatedKVVM)).To(Succeed())
		Expect(updatedKVVM.Spec.Template.Spec.Volumes).To(HaveLen(1))
		Expect(updatedKVVM.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal(sourcePVC))
	})

	It("reverts a migration PVC swap to the source even while restart is required", func() {
		ctx := testutil.ContextBackgroundWithNoOpLogger()

		vm := newVM()
		// KVVM/KVVMI are left pointing at a migration target PVC that must be
		// reverted back to the source. It is not a structural change (same disk),
		// so the revert must proceed despite the pending restart.
		kvvmInCluster := newKVVMWithVolume(targetPVC, nil, "target-node")
		kvvmi := newKVVMIWithVolume(targetPVC)
		desiredKVVM := newKVVMWithVolume(sourcePVC, nil, "source-node")
		vmState := setupState(vm, kvvmInCluster, kvvmi)

		service := NewMigrationVolumesService(
			vmState.Client(),
			func(context.Context, state.VirtualMachineState) (*virtv1.VirtualMachine, error) {
				return desiredKVVM.DeepCopy(), nil
			},
			10*time.Second,
		)

		_, err := service.SyncVolumes(ctx, vmState, true)
		Expect(err).NotTo(HaveOccurred())

		updatedKVVM := &virtv1.VirtualMachine{}
		Expect(vmState.Client().Get(ctx, types.NamespacedName{Name: vmName, Namespace: namespace}, updatedKVVM)).To(Succeed())
		Expect(updatedKVVM.Spec.Template.Spec.Volumes).To(HaveLen(1))
		Expect(updatedKVVM.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal(sourcePVC))
	})

	It("forces volume rollback when kvvmi is missing", func() {
		ctx := testutil.ContextBackgroundWithNoOpLogger()
		migrationStrategy := virtv1.UpdateVolumesStrategyMigration

		vm := newVM()
		kvvmInCluster := newKVVMWithVolume(targetPVC, &migrationStrategy, "target-node")
		desiredKVVM := newKVVMWithVolume(sourcePVC, nil, "source-node")
		vmState := setupState(vm, kvvmInCluster)

		service := NewMigrationVolumesService(
			vmState.Client(),
			func(context.Context, state.VirtualMachineState) (*virtv1.VirtualMachine, error) {
				return desiredKVVM.DeepCopy(), nil
			},
			10*time.Second,
		)

		_, err := service.SyncVolumes(ctx, vmState, false)
		Expect(err).NotTo(HaveOccurred())

		updatedKVVM := &virtv1.VirtualMachine{}
		Expect(vmState.Client().Get(ctx, types.NamespacedName{Name: vmName, Namespace: namespace}, updatedKVVM)).To(Succeed())
		Expect(updatedKVVM.Spec.UpdateVolumesStrategy).To(BeNil())
		Expect(updatedKVVM.Spec.Template.Spec.Volumes).To(HaveLen(1))
		Expect(updatedKVVM.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim).NotTo(BeNil())
		Expect(updatedKVVM.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal(sourcePVC))
		Expect(updatedKVVM.Spec.Template.Spec.Affinity).To(Equal(desiredKVVM.Spec.Template.Spec.Affinity))
	})

	It("clears a stale migration strategy left on kvvm when volumes already match and no migration is in progress", func() {
		ctx := testutil.ContextBackgroundWithNoOpLogger()
		migrationStrategy := virtv1.UpdateVolumesStrategyMigration

		vm := newVM()
		// A finished migration left updateVolumesStrategy=Migration on KVVM while KVVM
		// and KVVMI already agree on the volumes. The stale strategy must be cleared.
		kvvmInCluster := newKVVMWithVolume(targetPVC, &migrationStrategy, "node")
		kvvmi := newKVVMIWithVolume(targetPVC)
		desiredKVVM := newKVVMWithVolume(targetPVC, nil, "node")
		vmState := setupState(vm, kvvmInCluster, kvvmi)

		service := NewMigrationVolumesService(
			vmState.Client(),
			func(context.Context, state.VirtualMachineState) (*virtv1.VirtualMachine, error) {
				return desiredKVVM.DeepCopy(), nil
			},
			10*time.Second,
		)

		_, err := service.SyncVolumes(ctx, vmState, false)
		Expect(err).NotTo(HaveOccurred())

		updatedKVVM := &virtv1.VirtualMachine{}
		Expect(vmState.Client().Get(ctx, types.NamespacedName{Name: vmName, Namespace: namespace}, updatedKVVM)).To(Succeed())
		Expect(updatedKVVM.Spec.UpdateVolumesStrategy).To(BeNil())
		Expect(updatedKVVM.Spec.Template.Spec.Volumes).To(HaveLen(1))
		Expect(updatedKVVM.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal(targetPVC))
	})

	It("clears a stale migration strategy on a VM with a containerdisk volume despite the pull-policy drift", func() {
		ctx := testutil.ContextBackgroundWithNoOpLogger()
		migrationStrategy := virtv1.UpdateVolumesStrategyMigration

		containerDisk := func(policy corev1.PullPolicy) virtv1.Volume {
			return virtv1.Volume{
				Name: "cdrom",
				VolumeSource: virtv1.VolumeSource{
					ContainerDisk: &virtv1.ContainerDiskSource{Image: "registry.example.com/image:tag", ImagePullPolicy: policy},
				},
			}
		}

		vm := newVM()
		// The pull policy is defaulted by kubevirt only on the VMI: KVVM and the
		// desired spec carry an empty one. The stale strategy must still be cleared.
		kvvmInCluster := newKVVMWithVolume(targetPVC, &migrationStrategy, "node")
		kvvmInCluster.Spec.Template.Spec.Volumes = append(kvvmInCluster.Spec.Template.Spec.Volumes, containerDisk(""))
		kvvmi := newKVVMIWithVolume(targetPVC)
		kvvmi.Spec.Volumes = append(kvvmi.Spec.Volumes, containerDisk(corev1.PullIfNotPresent))
		desiredKVVM := newKVVMWithVolume(targetPVC, nil, "node")
		desiredKVVM.Spec.Template.Spec.Volumes = append(desiredKVVM.Spec.Template.Spec.Volumes, containerDisk(""))
		vmState := setupState(vm, kvvmInCluster, kvvmi)

		service := NewMigrationVolumesService(
			vmState.Client(),
			func(context.Context, state.VirtualMachineState) (*virtv1.VirtualMachine, error) {
				return desiredKVVM.DeepCopy(), nil
			},
			10*time.Second,
		)

		_, err := service.SyncVolumes(ctx, vmState, false)
		Expect(err).NotTo(HaveOccurred())

		updatedKVVM := &virtv1.VirtualMachine{}
		Expect(vmState.Client().Get(ctx, types.NamespacedName{Name: vmName, Namespace: namespace}, updatedKVVM)).To(Succeed())
		Expect(updatedKVVM.Spec.UpdateVolumesStrategy).To(BeNil())
	})

	It("force-reverts kvvm to source when kvvm/kvvmi diverged and no migration is in progress", func() {
		ctx := testutil.ContextBackgroundWithNoOpLogger()
		migrationStrategy := virtv1.UpdateVolumesStrategyMigration

		vm := newVM()
		// KVVM is stuck on a dead migration target (with the migration strategy),
		// while KVVMI never synced and still points at the source. With no in-progress
		// migration this must be force-reverted instead of waiting on the kvvmiSynced
		// barrier forever.
		kvvmInCluster := newKVVMWithVolume(targetPVC, &migrationStrategy, "target-node")
		kvvmi := newKVVMIWithVolume(sourcePVC)
		desiredKVVM := newKVVMWithVolume(sourcePVC, nil, "source-node")
		vmState := setupState(vm, kvvmInCluster, kvvmi)

		service := NewMigrationVolumesService(
			vmState.Client(),
			func(context.Context, state.VirtualMachineState) (*virtv1.VirtualMachine, error) {
				return desiredKVVM.DeepCopy(), nil
			},
			10*time.Second,
		)

		_, err := service.SyncVolumes(ctx, vmState, false)
		Expect(err).NotTo(HaveOccurred())

		updatedKVVM := &virtv1.VirtualMachine{}
		Expect(vmState.Client().Get(ctx, types.NamespacedName{Name: vmName, Namespace: namespace}, updatedKVVM)).To(Succeed())
		Expect(updatedKVVM.Spec.UpdateVolumesStrategy).To(BeNil())
		Expect(updatedKVVM.Spec.Template.Spec.Volumes).To(HaveLen(1))
		Expect(updatedKVVM.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal(sourcePVC))
	})

	It("does not revert diverged volumes when no migration strategy is set (e.g. hotplug mid-attach)", func() {
		ctx := testutil.ContextBackgroundWithNoOpLogger()

		vm := newVM()
		// KVVM and KVVMI diverge but there is no migration strategy: a benign
		// transient such as a hotplug volume being attached. It must be left to sync,
		// not force-reverted (that would tear down the in-flight attachment).
		kvvmInCluster := newKVVMWithVolume(targetPVC, nil, "node")
		kvvmi := newKVVMIWithVolume(sourcePVC)
		desiredKVVM := newKVVMWithVolume(sourcePVC, nil, "node")
		vmState := setupState(vm, kvvmInCluster, kvvmi)

		service := NewMigrationVolumesService(
			vmState.Client(),
			func(context.Context, state.VirtualMachineState) (*virtv1.VirtualMachine, error) {
				return desiredKVVM.DeepCopy(), nil
			},
			10*time.Second,
		)

		_, err := service.SyncVolumes(ctx, vmState, false)
		Expect(err).NotTo(HaveOccurred())

		updatedKVVM := &virtv1.VirtualMachine{}
		Expect(vmState.Client().Get(ctx, types.NamespacedName{Name: vmName, Namespace: namespace}, updatedKVVM)).To(Succeed())
		Expect(updatedKVVM.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal(targetPVC))
	})

	It("rejects a volume patch built from a stale kvvm read", func() {
		ctx := testutil.ContextBackgroundWithNoOpLogger()
		migrationStrategy := virtv1.UpdateVolumesStrategyMigration

		vm := newVM()
		kvvmInCluster := newKVVMWithVolume(targetPVC, &migrationStrategy, "target-node")
		kvvmi := newKVVMIWithVolume(sourcePVC)
		// The desired spec carries the resourceVersion of the kvvm it was built from;
		// if the kvvm changed since (e.g. kubevirt persisted a hotplug volume), the
		// patch must fail instead of overwriting the volumes from the stale read.
		desiredKVVM := newKVVMWithVolume(sourcePVC, nil, "source-node")
		desiredKVVM.ResourceVersion = "stale"
		vmState := setupState(vm, kvvmInCluster, kvvmi)

		service := NewMigrationVolumesService(
			vmState.Client(),
			func(context.Context, state.VirtualMachineState) (*virtv1.VirtualMachine, error) {
				return desiredKVVM.DeepCopy(), nil
			},
			10*time.Second,
		)

		_, err := service.SyncVolumes(ctx, vmState, false)
		Expect(err).To(HaveOccurred())

		updatedKVVM := &virtv1.VirtualMachine{}
		Expect(vmState.Client().Get(ctx, types.NamespacedName{Name: vmName, Namespace: namespace}, updatedKVVM)).To(Succeed())
		Expect(updatedKVVM.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal(targetPVC))
	})

	It("does not force-revert kvvm while a migration round without a vmop is active", func() {
		ctx := testutil.ContextBackgroundWithNoOpLogger()
		migrationStrategy := virtv1.UpdateVolumesStrategyMigration

		// A storage class change round runs without any VMOP: the disk is migrating,
		// KVVM is already patched with the round's target, KVVMI is not synced yet.
		// The diverged KVVM is the round starting, not a leftover to revert.
		vm := newVM()
		vm.Status.BlockDeviceRefs = []v1alpha2.BlockDeviceStatusRef{{Kind: v1alpha2.DiskDevice, Name: "root"}}
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "root", Namespace: namespace},
		}
		vd.Status.MigrationState.StartTimestamp = metav1.Now()
		vd.Status.MigrationState.TargetPVC = targetPVC
		kvvmInCluster := newKVVMWithVolume(targetPVC, &migrationStrategy, "node")
		kvvmi := newKVVMIWithVolume(sourcePVC)
		desiredKVVM := newKVVMWithVolume(sourcePVC, nil, "node")
		vmState := setupState(vm, kvvmInCluster, kvvmi, vd)

		service := NewMigrationVolumesService(
			vmState.Client(),
			func(context.Context, state.VirtualMachineState) (*virtv1.VirtualMachine, error) {
				return desiredKVVM.DeepCopy(), nil
			},
			10*time.Second,
		)

		_, err := service.SyncVolumes(ctx, vmState, false)
		Expect(err).NotTo(HaveOccurred())

		updatedKVVM := &virtv1.VirtualMachine{}
		Expect(vmState.Client().Get(ctx, types.NamespacedName{Name: vmName, Namespace: namespace}, updatedKVVM)).To(Succeed())
		Expect(updatedKVVM.Spec.UpdateVolumesStrategy).To(HaveValue(Equal(migrationStrategy)))
		Expect(updatedKVVM.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal(targetPVC))
	})
})

var _ = Describe("isStructuralVolumeChange", func() {
	// volumes builds a volume list from name -> claim pairs; the claim only
	// exists to prove that isStructuralVolumeChange ignores it and looks at names.
	volumes := func(nameToClaim map[string]string) []virtv1.Volume {
		vols := make([]virtv1.Volume, 0, len(nameToClaim))
		for name, claim := range nameToClaim {
			vols = append(vols, virtv1.Volume{
				Name: name,
				VolumeSource: virtv1.VolumeSource{
					PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: claim,
						},
					},
				},
			})
		}
		return vols
	}

	built := func(nameToClaim map[string]string) *virtv1.VirtualMachine {
		return &virtv1.VirtualMachine{
			Spec: virtv1.VirtualMachineSpec{
				Template: &virtv1.VirtualMachineInstanceTemplateSpec{
					Spec: virtv1.VirtualMachineInstanceSpec{Volumes: volumes(nameToClaim)},
				},
			},
		}
	}

	running := func(nameToClaim map[string]string) *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			Spec: virtv1.VirtualMachineInstanceSpec{Volumes: volumes(nameToClaim)},
		}
	}

	DescribeTable("distinguishes structural changes from PVC swaps",
		func(desired, current map[string]string, expected bool) {
			Expect(isStructuralVolumeChange(built(desired), running(current))).To(Equal(expected))
		},
		Entry("identical single disk", map[string]string{"root": "a"}, map[string]string{"root": "a"}, false),
		Entry("PVC swap on the same disk (migration/revert)", map[string]string{"root": "src"}, map[string]string{"root": "tgt"}, false),
		Entry("PVC swap on some of many disks", map[string]string{"root": "a", "data": "new"}, map[string]string{"root": "a", "data": "old"}, false),
		Entry("reordered volumes", map[string]string{"a": "1", "b": "2"}, map[string]string{"b": "2", "a": "1"}, false),
		Entry("both empty", map[string]string{}, map[string]string{}, false),
		Entry("disk added", map[string]string{"root": "a", "extra": "b"}, map[string]string{"root": "a"}, true),
		Entry("disk removed", map[string]string{"root": "a"}, map[string]string{"root": "a", "extra": "b"}, true),
		Entry("disk renamed (same count, different name)", map[string]string{"root": "a"}, map[string]string{"data": "a"}, true),
	)
})

var _ = Describe("allDisksMigrating", func() {
	mk := func(started, ended bool) *v1alpha2.VirtualDisk {
		vd := &v1alpha2.VirtualDisk{}
		if started {
			vd.Status.MigrationState.StartTimestamp = metav1.Now()
		}
		if ended {
			vd.Status.MigrationState.EndTimestamp = metav1.Now()
		}
		return vd
	}

	It("is true for an empty set", func() {
		Expect(allDisksMigrating(map[string]*v1alpha2.VirtualDisk{})).To(BeTrue())
	})
	It("is true when every disk is migrating this round", func() {
		Expect(allDisksMigrating(map[string]*v1alpha2.VirtualDisk{"a": mk(true, false), "b": mk(true, false)})).To(BeTrue())
	})
	It("is false when a disk has not started migrating", func() {
		Expect(allDisksMigrating(map[string]*v1alpha2.VirtualDisk{"a": mk(true, false), "b": mk(false, false)})).To(BeFalse())
	})
	It("is false when a disk already completed a previous round", func() {
		Expect(allDisksMigrating(map[string]*v1alpha2.VirtualDisk{"a": mk(true, false), "b": mk(true, true)})).To(BeFalse())
	})
})

var _ = Describe("isVolumeMigrating", func() {
	withVolumesChange := func(status corev1.ConditionStatus, set bool) *virtv1.VirtualMachineInstance {
		vmi := &virtv1.VirtualMachineInstance{}
		if set {
			vmi.Status.Conditions = []virtv1.VirtualMachineInstanceCondition{
				{Type: virtv1.VirtualMachineInstanceVolumesChange, Status: status},
			}
		}
		return vmi
	}

	It("is true when VolumesChange condition is True", func() {
		Expect(isVolumeMigrating(withVolumesChange(corev1.ConditionTrue, true))).To(BeTrue())
	})
	It("is false when VolumesChange condition is False", func() {
		Expect(isVolumeMigrating(withVolumesChange(corev1.ConditionFalse, true))).To(BeFalse())
	})
	It("is false when VolumesChange condition is absent", func() {
		Expect(isVolumeMigrating(withVolumesChange(corev1.ConditionTrue, false))).To(BeFalse())
	})
})

var _ = Describe("destinationsMatch", func() {
	pvcVolumes := func(nameToClaim map[string]string) []virtv1.Volume {
		vols := make([]virtv1.Volume, 0, len(nameToClaim))
		for name, claim := range nameToClaim {
			vols = append(vols, virtv1.Volume{
				Name:         name,
				VolumeSource: virtv1.VolumeSource{PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim}}},
			})
		}
		return vols
	}
	built := func(nameToClaim map[string]string) *virtv1.VirtualMachine {
		return &virtv1.VirtualMachine{Spec: virtv1.VirtualMachineSpec{Template: &virtv1.VirtualMachineInstanceTemplateSpec{Spec: virtv1.VirtualMachineInstanceSpec{Volumes: pvcVolumes(nameToClaim)}}}}
	}
	kvvmi := func(running, volToDest map[string]string) *virtv1.VirtualMachineInstance {
		vmi := &virtv1.VirtualMachineInstance{}
		vmi.Spec.Volumes = pvcVolumes(running)
		for vol, dest := range volToDest {
			vmi.Status.MigratedVolumes = append(vmi.Status.MigratedVolumes, virtv1.StorageMigratedVolumeInfo{
				VolumeName:         vol,
				DestinationPVCInfo: &virtv1.PersistentVolumeClaimInfo{ClaimName: dest},
			})
		}
		return vmi
	}

	It("is true when the set keeps the running claims and there is no recorded migration", func() {
		Expect(destinationsMatch(kvvmi(map[string]string{"root": "src"}, nil), built(map[string]string{"root": "src"}))).To(BeTrue())
	})
	It("is false when a claim changes without a recorded migration for it", func() {
		Expect(destinationsMatch(kvvmi(map[string]string{"root": "src"}, nil), built(map[string]string{"root": "new"}))).To(BeFalse())
	})
	It("is true when the recorded destination matches the target being patched", func() {
		Expect(destinationsMatch(kvvmi(map[string]string{"root": "src"}, map[string]string{"root": "tgt"}), built(map[string]string{"root": "tgt"}))).To(BeTrue())
	})
	It("is false when the recorded destination differs from the new target", func() {
		Expect(destinationsMatch(kvvmi(map[string]string{"root": "src"}, map[string]string{"root": "old-tgt"}), built(map[string]string{"root": "new-tgt"}))).To(BeFalse())
	})
	It("is false when the set migrates an extra volume on top of the in-flight round", func() {
		Expect(destinationsMatch(
			kvvmi(map[string]string{"root": "src", "data": "data-src"}, map[string]string{"root": "tgt"}),
			built(map[string]string{"root": "tgt", "data": "data-tgt"}),
		)).To(BeFalse())
	})
	It("is false when the set keeps a volume at source while it is migrating", func() {
		Expect(destinationsMatch(
			kvvmi(map[string]string{"root": "src", "data": "data-src"}, map[string]string{"root": "tgt", "data": "data-tgt"}),
			built(map[string]string{"root": "tgt", "data": "data-src"}),
		)).To(BeFalse())
	})
	It("is false when an in-flight volume is absent from the set being patched", func() {
		Expect(destinationsMatch(
			kvvmi(map[string]string{"root": "src"}, map[string]string{"hotplug": "hp-tgt"}),
			built(map[string]string{"root": "src"}),
		)).To(BeFalse())
	})
	It("ignores recorded entries without destination info", func() {
		vmi := kvvmi(map[string]string{"root": "src"}, nil)
		vmi.Status.MigratedVolumes = []virtv1.StorageMigratedVolumeInfo{{VolumeName: "root", DestinationPVCInfo: nil}}
		Expect(destinationsMatch(vmi, built(map[string]string{"root": "src"}))).To(BeTrue())
	})
})
