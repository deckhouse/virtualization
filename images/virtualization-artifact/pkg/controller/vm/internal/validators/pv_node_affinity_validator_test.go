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
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/validators"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("PVNodeAffinityValidator", func() {
	const (
		ns    = "test-ns"
		node1 = "node-1"
		node2 = "node-2"
	)

	makeNode := func(name string, taint ...corev1.Taint) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: map[string]string{"topology.kubernetes.io/node": name},
			},
			Spec: corev1.NodeSpec{Taints: taint},
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

	makeVM := func(nodeName string, refs ...v1alpha2.BlockDeviceSpecRef) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: ns},
			Spec: v1alpha2.VirtualMachineSpec{
				BlockDeviceRefs:          refs,
				VirtualMachineClassName:  "generic",
			},
			Status: v1alpha2.VirtualMachineStatus{Node: nodeName},
		}
	}

	makeVMClass := func() *v1alpha2.VirtualMachineClass {
		return &v1alpha2.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{Name: "generic"},
		}
	}

	makeKVVMI := func() *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: ns},
			Status:     virtv1.VirtualMachineInstanceStatus{NodeName: node1},
		}
	}

	makeValidator := func(objs ...client.Object) *validators.PVNodeAffinityValidator {
		fakeClient := setupEnvironment(objs...)
		attacher := service.NewAttachmentService(fakeClient, nil, "")
		return validators.NewPVNodeAffinityValidator(fakeClient, attacher)
	}

	Context("scheduled VM", func() {
		It("should allow when blockDeviceRefs unchanged", func() {
			refs := []v1alpha2.BlockDeviceSpecRef{{Kind: v1alpha2.DiskDevice, Name: "disk1"}}
			oldVM := makeVM(node1, refs...)
			newVM := makeVM(node1, refs...)
			v := makeValidator(oldVM, makeNode(node1))
			_, err := v.ValidateUpdate(testutil.ContextBackgroundWithNoOpLogger(), oldVM, newVM)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("should allow adding a network disk", func() {
			oldVM := makeVM(node1, v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "disk1"})
			newVM := makeVM(node1,
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "disk1"},
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "net-disk"},
			)
			v := makeValidator(
				oldVM, makeNode(node1), makeKVVMI(),
				makeVD("net-disk", "pvc-net"),
				makePVC("pvc-net", "pv-net"),
				makePV("pv-net"),
			)
			_, err := v.ValidateUpdate(testutil.ContextBackgroundWithNoOpLogger(), oldVM, newVM)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("should allow adding a local disk available on VM node", func() {
			oldVM := makeVM(node1, v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "disk1"})
			newVM := makeVM(node1,
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "disk1"},
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "local-disk"},
			)
			v := makeValidator(
				oldVM, makeNode(node1), makeKVVMI(),
				makeVD("local-disk", "pvc-local"),
				makePVC("pvc-local", "pv-local"),
				makePV("pv-local", node1),
			)
			_, err := v.ValidateUpdate(testutil.ContextBackgroundWithNoOpLogger(), oldVM, newVM)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("should reject adding a local disk NOT available on VM node", func() {
			oldVM := makeVM(node1, v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "disk1"})
			newVM := makeVM(node1,
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "disk1"},
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "local-disk"},
			)
			v := makeValidator(
				oldVM, makeNode(node1), makeKVVMI(),
				makeVD("local-disk", "pvc-local"),
				makePVC("pvc-local", "pv-local"),
				makePV("pv-local", node2),
			)
			_, err := v.ValidateUpdate(testutil.ContextBackgroundWithNoOpLogger(), oldVM, newVM)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("unable to attach disks"))
			Expect(err.Error()).Should(ContainSubstring("local-disk"))
		})

		It("should list all incompatible disks in error message", func() {
			oldVM := makeVM(node1)
			newVM := makeVM(node1,
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "bad-1"},
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "bad-2"},
			)
			v := makeValidator(
				oldVM, makeNode(node1), makeKVVMI(),
				makeVD("bad-1", "pvc-1"), makePVC("pvc-1", "pv-1"), makePV("pv-1", node2),
				makeVD("bad-2", "pvc-2"), makePVC("pvc-2", "pv-2"), makePV("pv-2", node2),
			)
			_, err := v.ValidateUpdate(testutil.ContextBackgroundWithNoOpLogger(), oldVM, newVM)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("bad-1"))
			Expect(err.Error()).Should(ContainSubstring("bad-2"))
		})

		It("should allow adding a disk with pending PVC", func() {
			oldVM := makeVM(node1)
			newVM := makeVM(node1,
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "new-disk"},
			)
			v := makeValidator(
				oldVM, makeNode(node1), makeKVVMI(),
				makeVD("new-disk", "pvc-pending"),
				&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{Name: "pvc-pending", Namespace: ns},
				},
			)
			_, err := v.ValidateUpdate(testutil.ContextBackgroundWithNoOpLogger(), oldVM, newVM)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	Context("unscheduled VM", func() {
		It("should allow create when topology is compatible", func() {
			vm := makeVM("",
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "local-disk"},
			)
			v := makeValidator(
				vm, makeNode(node1), makeVMClass(),
				makeVD("local-disk", "pvc-local"),
				makePVC("pvc-local", "pv-local"),
				makePV("pv-local", node1),
			)
			_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vm)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("should reject create when no node satisfies all constraints", func() {
			vm := makeVM("",
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "disk-a"},
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "disk-b"},
			)
			v := makeValidator(
				vm, makeNode(node1), makeNode(node2), makeVMClass(),
				makeVD("disk-a", "pvc-a"), makePVC("pvc-a", "pv-a"), makePV("pv-a", node1),
				makeVD("disk-b", "pvc-b"), makePVC("pvc-b", "pv-b"), makePV("pv-b", node2),
			)
			_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vm)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("unable to create"))
			Expect(err.Error()).Should(ContainSubstring("topology conflict"))
		})

		It("should reject update when no node satisfies all constraints", func() {
			oldVM := makeVM("")
			newVM := makeVM("",
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "disk-a"},
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "disk-b"},
			)
			v := makeValidator(
				newVM, makeNode(node1), makeNode(node2), makeVMClass(),
				makeVD("disk-a", "pvc-a"), makePVC("pvc-a", "pv-a"), makePV("pv-a", node1),
				makeVD("disk-b", "pvc-b"), makePVC("pvc-b", "pv-b"), makePV("pv-b", node2),
			)
			_, err := v.ValidateUpdate(testutil.ContextBackgroundWithNoOpLogger(), oldVM, newVM)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("unable to update"))
		})

		It("should allow when disks have no PV nodeAffinity (network storage)", func() {
			vm := makeVM("",
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "net-disk"},
			)
			v := makeValidator(
				vm, makeNode(node1), makeVMClass(),
				makeVD("net-disk", "pvc-net"),
				makePVC("pvc-net", "pv-net"),
				makePV("pv-net"),
			)
			_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vm)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("should reject when PV node conflicts with VMClass nodeSelector", func() {
			vm := makeVM("",
				v1alpha2.BlockDeviceSpecRef{Kind: v1alpha2.DiskDevice, Name: "local-disk"},
			)
			vmClass := &v1alpha2.VirtualMachineClass{
				ObjectMeta: metav1.ObjectMeta{Name: "generic"},
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
				vm, makeNode(node1), makeNode(node2), vmClass,
				makeVD("local-disk", "pvc-local"),
				makePVC("pvc-local", "pv-local"),
				makePV("pv-local", node1), // disk on node1, class requires node2
			)
			_, err := v.ValidateCreate(testutil.ContextBackgroundWithNoOpLogger(), vm)
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("topology conflict"))
		})
	})
})
