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

package vd

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestCommonVD(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Common VD")
}

var _ = Describe("GetDVCRNodePlacement", func() {
	const ns = "default"

	systemToleration := corev1.Toleration{
		Key:      "dedicated.deckhouse.io",
		Operator: corev1.TolerationOpEqual,
		Value:    "system",
	}
	vmToleration := corev1.Toleration{
		Key:      "dedicated.deckhouse.io",
		Operator: corev1.TolerationOpEqual,
		Value:    "vm-workloads",
		Effect:   corev1.TaintEffectNoSchedule,
	}
	vmClassToleration := corev1.Toleration{
		Key:      "dedicated.deckhouse.io",
		Operator: corev1.TolerationOpEqual,
		Value:    "vm-class",
		Effect:   corev1.TaintEffectNoSchedule,
	}

	newClient := func(objs ...client.Object) client.Client {
		scheme := runtime.NewScheme()
		Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	}

	newVD := func(attachedVMs ...string) *v1alpha2.VirtualDisk {
		vd := &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd", Namespace: ns},
		}
		for _, name := range attachedVMs {
			vd.Status.AttachedToVirtualMachines = append(vd.Status.AttachedToVirtualMachines, v1alpha2.AttachedVirtualMachine{Name: name})
		}
		return vd
	}

	newVM := func(tolerations ...corev1.Toleration) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: ns},
			Spec: v1alpha2.VirtualMachineSpec{
				VirtualMachineClassName: "vmclass",
				CPU:                     v1alpha2.CPUSpec{Cores: 2},
				Tolerations:             tolerations,
			},
			Status: v1alpha2.VirtualMachineStatus{Node: "node-1"},
		}
	}

	newVMClass := func(tolerations ...corev1.Toleration) *v1alpha2.VirtualMachineClass {
		return &v1alpha2.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{Name: "vmclass"},
			Spec:       v1alpha2.VirtualMachineClassSpec{Tolerations: tolerations},
		}
	}

	It("returns only the system toleration when no vm is attached", func() {
		nodePlacement, err := GetDVCRNodePlacement(context.Background(), newClient(), newVD())
		Expect(err).ToNot(HaveOccurred())
		Expect(nodePlacement).ToNot(BeNil())
		Expect(nodePlacement.Tolerations).To(Equal([]corev1.Toleration{systemToleration}))
	})

	It("returns only the system toleration when more than one vm is attached", func() {
		nodePlacement, err := GetDVCRNodePlacement(context.Background(), newClient(), newVD("vm-a", "vm-b"))
		Expect(err).ToNot(HaveOccurred())
		Expect(nodePlacement).ToNot(BeNil())
		Expect(nodePlacement.Tolerations).To(Equal([]corev1.Toleration{systemToleration}))
	})

	It("returns only the system toleration when the attached vm does not exist", func() {
		nodePlacement, err := GetDVCRNodePlacement(context.Background(), newClient(), newVD("vm"))
		Expect(err).ToNot(HaveOccurred())
		Expect(nodePlacement).ToNot(BeNil())
		Expect(nodePlacement.Tolerations).To(Equal([]corev1.Toleration{systemToleration}))
	})

	It("appends vm, vm class and system tolerations in order", func() {
		c := newClient(newVM(vmToleration), newVMClass(vmClassToleration))

		nodePlacement, err := GetDVCRNodePlacement(context.Background(), c, newVD("vm"))
		Expect(err).ToNot(HaveOccurred())
		Expect(nodePlacement).ToNot(BeNil())
		Expect(nodePlacement.Tolerations).To(Equal([]corev1.Toleration{vmToleration, vmClassToleration, systemToleration}))
		Expect(nodePlacement.Node).To(Equal("node-1"))
		Expect(nodePlacement.CPUCores).To(Equal(2))
	})

	It("does not duplicate the system toleration when the vm already has it", func() {
		c := newClient(newVM(systemToleration), newVMClass())

		nodePlacement, err := GetDVCRNodePlacement(context.Background(), c, newVD("vm"))
		Expect(err).ToNot(HaveOccurred())
		Expect(nodePlacement).ToNot(BeNil())
		Expect(nodePlacement.Tolerations).To(Equal([]corev1.Toleration{systemToleration}))
	})

	It("keeps the catch-all system toleration when the vm has a narrower one for the same key and value", func() {
		narrowerToleration := corev1.Toleration{
			Key:      "dedicated.deckhouse.io",
			Operator: corev1.TolerationOpEqual,
			Value:    "system",
			Effect:   corev1.TaintEffectNoExecute,
		}
		c := newClient(newVM(narrowerToleration), newVMClass())

		nodePlacement, err := GetDVCRNodePlacement(context.Background(), c, newVD("vm"))
		Expect(err).ToNot(HaveOccurred())
		Expect(nodePlacement).ToNot(BeNil())
		Expect(nodePlacement.Tolerations).To(Equal([]corev1.Toleration{narrowerToleration, systemToleration}))
	})

	DescribeTable(
		"produces the same tolerations hash on create and on change detection",
		func(vd *v1alpha2.VirtualDisk, objs ...client.Object) {
			c := newClient(objs...)

			createSide, err := GetDVCRNodePlacement(context.Background(), c, vd)
			Expect(err).ToNot(HaveOccurred())

			pod := &corev1.Pod{}
			Expect(provisioner.KeepNodePlacementTolerations(createSide, pod)).To(Succeed())

			waitSide, err := GetDVCRNodePlacement(context.Background(), c, vd)
			Expect(err).ToNot(HaveOccurred())

			isChanged, err := provisioner.IsNodePlacementChanged(waitSide, pod)
			Expect(err).ToNot(HaveOccurred())
			Expect(isChanged).To(BeFalse())
		},
		Entry("without attached vm", newVD()),
		Entry("with attached vm and tolerations", newVD("vm"), newVM(vmToleration), newVMClass(vmClassToleration)),
	)
})
