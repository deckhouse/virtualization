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

var _ = Describe("sameDiskNameSet", func() {
	withDisks := func(names ...string) *virtv1.VirtualMachine {
		disks := make([]virtv1.Disk, 0, len(names))
		for _, n := range names {
			disks = append(disks, virtv1.Disk{Name: n})
		}
		return &virtv1.VirtualMachine{
			Spec: virtv1.VirtualMachineSpec{
				Template: &virtv1.VirtualMachineInstanceTemplateSpec{
					Spec: virtv1.VirtualMachineInstanceSpec{
						Domain: virtv1.DomainSpec{Devices: virtv1.Devices{Disks: disks}},
					},
				},
			},
		}
	}

	DescribeTable("compares disk name sets regardless of order",
		func(built, live []string, expected bool) {
			Expect(sameDiskNameSet(withDisks(built...), withDisks(live...))).To(Equal(expected))
		},
		Entry("identical", []string{"root", "data"}, []string{"root", "data"}, true),
		Entry("reordered", []string{"root", "data"}, []string{"data", "root"}, true),
		Entry("both empty", []string{}, []string{}, true),
		Entry("live has an extra hotplug disk not yet in the built spec", []string{"root", "data"}, []string{"root", "data", "vd-vd-vmbda-rwo"}, false),
		Entry("built has a disk the live KVVM lost", []string{"root", "data"}, []string{"root"}, false),
		Entry("same count, different name", []string{"root"}, []string{"data"}, false),
	)
})

var _ = Describe("GetVirtualDiskNamesWithUnreadyTarget", func() {
	const namespace = "default"

	newPVC := func(name string, phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolumeClaim"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: phase},
		}
	}

	newVD := func(name, sourcePVC, targetPVC string) *v1alpha2.VirtualDisk {
		vd := &v1alpha2.VirtualDisk{
			TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualDiskKind},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		}
		vd.Status.Target.PersistentVolumeClaim = sourcePVC
		if targetPVC != "" {
			vd.Status.MigrationState = v1alpha2.VirtualDiskMigrationState{
				SourcePVC:      sourcePVC,
				TargetPVC:      targetPVC,
				StartTimestamp: metav1.Now(),
			}
		}
		return vd
	}

	It("reports a disk that has not started migrating as not ready", func() {
		ctx := testutil.ContextBackgroundWithNoOpLogger()

		vm := &v1alpha2.VirtualMachine{
			TypeMeta:   metav1.TypeMeta{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualMachineKind},
			ObjectMeta: metav1.ObjectMeta{Name: "vm-unready", Namespace: namespace, Generation: 1},
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs: []v1alpha2.BlockDeviceSpecRef{
					{Kind: v1alpha2.DiskDevice, Name: "vd-migrating"},
					{Kind: v1alpha2.DiskDevice, Name: "vd-pending"},
				},
			},
		}

		// vd-migrating: migration started and its target PVC is bound -> ready.
		vdMigrating := newVD("vd-migrating", "migrating-source", "migrating-target")
		// vd-pending: no migration target yet, only its source PVC is bound. Before
		// the fix this was falsely counted as ready (readiness was read from
		// Status.Target, i.e. the source), which let a partial migration set be
		// applied to the KVVM. It must now be reported as not ready so the operation
		// waits for every disk to join the migration set.
		vdPending := newVD("vd-pending", "pending-source", "")

		fakeClient, err := testutil.NewFakeClientWithObjects(
			vm, vdMigrating, vdPending,
			newPVC("migrating-source", corev1.ClaimBound),
			newPVC("migrating-target", corev1.ClaimBound),
			newPVC("pending-source", corev1.ClaimBound),
		)
		Expect(err).NotTo(HaveOccurred())

		resource := reconciler.NewResource(client.ObjectKeyFromObject(vm), fakeClient,
			func() *v1alpha2.VirtualMachine { return &v1alpha2.VirtualMachine{} },
			func(obj *v1alpha2.VirtualMachine) v1alpha2.VirtualMachineStatus { return obj.Status },
		)
		Expect(resource.Fetch(context.Background())).To(Succeed())
		vmState := state.New(fakeClient, resource)

		svc := NewMigrationVolumesService(fakeClient,
			func(context.Context, state.VirtualMachineState) (*virtv1.VirtualMachine, error) { return nil, nil },
			10*time.Second,
		)

		notReady, err := svc.GetVirtualDiskNamesWithUnreadyTarget(ctx, vmState)
		Expect(err).NotTo(HaveOccurred())
		Expect(notReady).To(ConsistOf("vd-pending"))
	})
})
