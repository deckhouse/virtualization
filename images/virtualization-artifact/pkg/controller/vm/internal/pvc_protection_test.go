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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("PVC protection", func() {
	const ns = "default"

	newScheme := func() *apiruntime.Scheme {
		scheme := apiruntime.NewScheme()
		for _, f := range []func(*apiruntime.Scheme) error{
			v1alpha2.AddToScheme,
			virtv1.AddToScheme,
			corev1.AddToScheme,
		} {
			Expect(f(scheme)).To(Succeed())
		}
		return scheme
	}

	newVM := func(name string, phase v1alpha2.MachinePhase) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Status:     v1alpha2.VirtualMachineStatus{Phase: phase},
		}
	}

	newPVC := func(name string, finalizers ...string) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Finalizers: finalizers},
		}
	}

	newKVVM := func(name string, claimNames ...string) *virtv1.VirtualMachine {
		volumes := make([]virtv1.Volume, 0, len(claimNames))
		for _, claimName := range claimNames {
			volumes = append(volumes, virtv1.Volume{
				Name: "vd-" + claimName,
				VolumeSource: virtv1.VolumeSource{
					PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
						PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: claimName,
						},
					},
				},
			})
		}
		return &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: virtv1.VirtualMachineSpec{
				Template: &virtv1.VirtualMachineInstanceTemplateSpec{
					Spec: virtv1.VirtualMachineInstanceSpec{Volumes: volumes},
				},
			},
		}
	}

	reconcile := func(objs ...client.Object) client.Client {
		fakeClient := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(objs...).Build()
		protection := service.NewProtectionService(fakeClient, v1alpha2.FinalizerPVCProtection)
		Expect(reconcilePVCProtection(context.Background(), fakeClient, protection, ns)).To(Succeed())
		return fakeClient
	}

	hasProtection := func(cl client.Client, pvcName string) bool {
		pvc := &corev1.PersistentVolumeClaim{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: pvcName, Namespace: ns}, pvc)).To(Succeed())
		return controllerutil.ContainsFinalizer(pvc, v1alpha2.FinalizerPVCProtection)
	}

	DescribeTable("vmRequiresPVCProtection",
		func(vm *v1alpha2.VirtualMachine, expected bool) {
			Expect(vmRequiresPVCProtection(vm)).To(Equal(expected))
		},
		Entry("nil VM", nil, false),
		Entry("pending VM", newVM("vm", v1alpha2.MachinePending), false),
		Entry("stopped VM", newVM("vm", v1alpha2.MachineStopped), false),
		Entry("starting VM", newVM("vm", v1alpha2.MachineStarting), true),
		Entry("running VM", newVM("vm", v1alpha2.MachineRunning), true),
		Entry("stopping VM", newVM("vm", v1alpha2.MachineStopping), true),
		Entry("terminating VM", newVM("vm", v1alpha2.MachineTerminating), true),
		Entry("migrating VM", newVM("vm", v1alpha2.MachineMigrating), true),
		Entry("paused VM", newVM("vm", v1alpha2.MachinePause), true),
		Entry("degraded VM", newVM("vm", v1alpha2.MachineDegraded), true),
		Entry("deleting stopped VM", &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "vm",
				Namespace:         ns,
				DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
				Finalizers:        []string{v1alpha2.FinalizerVMCleanup},
			},
			Status: v1alpha2.VirtualMachineStatus{Phase: v1alpha2.MachineStopped},
		}, true),
	)

	It("collects claim names from KVVM template, KVVMI spec and KVVMI volumeStatus", func() {
		kvvmi := &virtv1.VirtualMachineInstance{
			Spec: virtv1.VirtualMachineInstanceSpec{
				Volumes: []virtv1.Volume{
					{
						VolumeSource: virtv1.VolumeSource{
							PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "claim-from-kvvmi-spec",
								},
							},
						},
					},
					{
						VolumeSource: virtv1.VolumeSource{ContainerDisk: &virtv1.ContainerDiskSource{}},
					},
				},
			},
			Status: virtv1.VirtualMachineInstanceStatus{
				VolumeStatus: []virtv1.VolumeStatus{
					{
						Name: "vd-unmounting",
						PersistentVolumeClaimInfo: &virtv1.PersistentVolumeClaimInfo{
							ClaimName: "claim-from-volume-status",
						},
					},
				},
			},
		}

		claims := make(map[string]struct{})
		volumeClaimNames(claims, newKVVM("vm", "claim-from-kvvm"), kvvmi)
		Expect(claims).To(HaveLen(3))
		Expect(claims).To(HaveKey("claim-from-kvvm"))
		Expect(claims).To(HaveKey("claim-from-kvvmi-spec"))
		Expect(claims).To(HaveKey("claim-from-volume-status"))
	})

	It("protects the PVC of a running VM and does not touch unrelated PVCs", func() {
		cl := reconcile(
			newVM("vm-running", v1alpha2.MachineRunning),
			newKVVM("vm-running", "vd-claim"),
			newPVC("vd-claim"),
			newPVC("unrelated-claim"),
		)
		Expect(hasProtection(cl, "vd-claim")).To(BeTrue())
		Expect(hasProtection(cl, "unrelated-claim")).To(BeFalse())
	})

	It("releases the PVC once the VM is stopped and its volume is gone from the KVVM", func() {
		cl := reconcile(
			newVM("vm-stopped", v1alpha2.MachineStopped),
			newKVVM("vm-stopped", "vd-claim"),
			newPVC("vd-claim", v1alpha2.FinalizerPVCProtection),
		)
		Expect(hasProtection(cl, "vd-claim")).To(BeFalse())
	})

	It("keeps the PVC of a terminating VM until the KVVMI volumeStatus drops the volume", func() {
		kvvmi := &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "vm-terminating", Namespace: ns},
			Status: virtv1.VirtualMachineInstanceStatus{
				VolumeStatus: []virtv1.VolumeStatus{
					{
						Name: "vd-hotplug",
						PersistentVolumeClaimInfo: &virtv1.PersistentVolumeClaimInfo{
							ClaimName: "hotplug-claim",
						},
					},
				},
			},
		}
		cl := reconcile(
			newVM("vm-terminating", v1alpha2.MachineTerminating),
			kvvmi,
			newPVC("hotplug-claim", v1alpha2.FinalizerPVCProtection),
		)
		Expect(hasProtection(cl, "hotplug-claim")).To(BeTrue())
	})

	It("releases the PVC of a terminating VM whose KVVM and KVVMI are gone", func() {
		cl := reconcile(
			newVM("vm-terminating", v1alpha2.MachineTerminating),
			newPVC("vd-claim", v1alpha2.FinalizerPVCProtection),
		)
		Expect(hasProtection(cl, "vd-claim")).To(BeFalse())
	})

	It("keeps a shared PVC while another VM still uses it", func() {
		cl := reconcile(
			newVM("vm-stopped", v1alpha2.MachineStopped),
			newKVVM("vm-stopped", "shared-claim"),
			newVM("vm-running", v1alpha2.MachineRunning),
			newKVVM("vm-running", "shared-claim"),
			newPVC("shared-claim", v1alpha2.FinalizerPVCProtection),
		)
		Expect(hasProtection(cl, "shared-claim")).To(BeTrue())
	})
})
