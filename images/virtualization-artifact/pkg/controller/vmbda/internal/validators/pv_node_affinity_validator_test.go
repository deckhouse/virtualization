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

package validators_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmbda/internal/validators"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("PVNodeAffinityValidator (VMBDA)", func() {
	const (
		ns    = "test-ns"
		node1 = "node-1"
		node2 = "node-2"
	)

	makeNode := func(name string) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: map[string]string{"topology.kubernetes.io/node": name},
			},
		}
	}

	makePV := func(name string, nodeNames ...string) *corev1.PersistentVolume {
		pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: name}}
		if len(nodeNames) > 0 {
			pv.Spec.NodeAffinity = &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      "topology.kubernetes.io/node",
							Operator: corev1.NodeSelectorOpIn,
							Values:   nodeNames,
						}},
					}},
				},
			}
		}
		return pv
	}

	makePVC := func(name, pvName string) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: pvName},
		}
	}

	makeVD := func(name, pvcName string) *v1alpha2.VirtualDisk {
		return &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Status:     v1alpha2.VirtualDiskStatus{Target: v1alpha2.DiskTarget{PersistentVolumeClaim: pvcName}},
		}
	}

	makeVM := func(nodeName string) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: ns},
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "generic",
			},
			Status: v1alpha2.VirtualMachineStatus{Node: nodeName},
		}
	}

	makeVMClass := func() *v1alpha2.VirtualMachineClass {
		return &v1alpha2.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{Name: "generic"},
		}
	}

	makeVMBDA := func(vdName string) *v1alpha2.VirtualMachineBlockDeviceAttachment {
		return &v1alpha2.VirtualMachineBlockDeviceAttachment{
			ObjectMeta: metav1.ObjectMeta{Name: "vmbda", Namespace: ns},
			Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
				VirtualMachineName: "vm",
				BlockDeviceRef: v1alpha2.VMBDAObjectRef{
					Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
					Name: vdName,
				},
			},
		}
	}

	makeValidator := func(objs ...client.Object) *validators.PVNodeAffinityValidator {
		fakeClient, err := testutil.NewFakeClientWithObjects(objs...)
		Expect(err).NotTo(HaveOccurred())
		attacher := service.NewAttachmentService(fakeClient, nil, "")
		return validators.NewPVNodeAffinityValidator(fakeClient, attacher)
	}

	Context("unscheduled VM", func() {
		It("allows create when disk PV has no node affinity (network storage)", func() {
			vm := makeVM("")
			vmbda := makeVMBDA("net-disk")
			v := makeValidator(
				vm, makeVMClass(), makeNode(node1),
				makeVD("net-disk", "pvc-net"),
				makePVC("pvc-net", "pv-net"),
				makePV("pv-net"),
			)
			_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vmbda)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("allows create when PV topology is compatible with VM placement", func() {
			vm := makeVM("")
			vmbda := makeVMBDA("local-disk")
			v := makeValidator(
				vm, makeVMClass(), makeNode(node1),
				makeVD("local-disk", "pvc-local"),
				makePVC("pvc-local", "pv-local"),
				makePV("pv-local", node1),
			)
			_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vmbda)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("rejects create when PV topology conflicts with VM class node selector", func() {
			vm := makeVM("")
			vm.Spec.VirtualMachineClassName = "restricted"
			vmbda := makeVMBDA("local-disk")
			vmClass := &v1alpha2.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{Name: "restricted"},
				Spec: v1alpha2.VirtualMachineClassSpec{
					NodeSelector: v1alpha2.NodeSelector{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      "topology.kubernetes.io/node",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{node2},
						}},
					},
				},
			}
			v := makeValidator(
				vm, vmClass, makeNode(node1), makeNode(node2),
				makeVD("local-disk", "pvc-local"),
				makePVC("pvc-local", "pv-local"),
				makePV("pv-local", node1),
			)
			_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vmbda)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("topology conflict"))
		})

		It("allows when VM class is not found", func() {
			vm := makeVM("")
			vmbda := makeVMBDA("local-disk")
			v := makeValidator(
				vm, makeNode(node1),
				makeVD("local-disk", "pvc-local"),
				makePVC("pvc-local", "pv-local"),
				makePV("pv-local", node1),
			)
			_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vmbda)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("allows when PVC is pending (no volume name)", func() {
			vm := makeVM("")
			vmbda := makeVMBDA("pending-disk")
			v := makeValidator(
				vm, makeVMClass(), makeNode(node1),
				makeVD("pending-disk", "pvc-pending"),
				&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{Name: "pvc-pending", Namespace: ns},
				},
			)
			_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vmbda)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("allows when referenced PV is missing", func() {
			vm := makeVM("")
			vmbda := makeVMBDA("local-disk")
			v := makeValidator(
				vm, makeVMClass(), makeNode(node1),
				makeVD("local-disk", "pvc-local"),
				makePVC("pvc-local", "pv-missing"),
			)
			_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vmbda)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("allows when referenced VM is missing", func() {
			vmbda := makeVMBDA("local-disk")
			v := makeValidator(
				makeVMClass(), makeNode(node1),
				makeVD("local-disk", "pvc-local"),
				makePVC("pvc-local", "pv-local"),
				makePV("pv-local", node1),
			)
			_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vmbda)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})
