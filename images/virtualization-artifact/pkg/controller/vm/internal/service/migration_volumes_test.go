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

	It("does not apply structural volume changes to kvvm while restart is required", func() {
		ctx := testutil.ContextBackgroundWithNoOpLogger()

		vm := newVM()
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
		Expect(updatedKVVM.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal(targetPVC))
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
