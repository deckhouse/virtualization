/*
Copyright 2024 Flant JSC

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

package vm

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestCalculateCoresAndSockets(t *testing.T) {
	tests := []struct {
		desiredCores   int
		sockets        int
		cores          int
		coresPerSocket int
	}{
		{-1, 1, 1, 16},
		{1, 1, 1, 16},
		{2, 1, 2, 16},
		{3, 1, 3, 16},
		{4, 1, 4, 16},
		{5, 1, 5, 16},
		{15, 1, 15, 16},
		{16, 1, 16, 16},

		{18, 2, 9, 16},
		{19, 2, 10, 16},
		{20, 2, 10, 16},
		{31, 2, 16, 16},
		{32, 2, 16, 16},

		{36, 4, 9, 16},
		{37, 4, 10, 16},
		{40, 4, 10, 16},
		{60, 4, 15, 16},
		{63, 4, 16, 16},
		{64, 4, 16, 16},

		{72, 8, 9, 31},
		{76, 8, 10, 31},
		{80, 8, 10, 31},
		{248, 8, 31, 31},
		{252, 8, 32, 31},
		{256, 8, 32, 31},
	}

	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			sockets, cores, coresPerSocket := CalculateCoresAndSockets(test.desiredCores)
			if cores != test.cores && sockets != test.sockets {
				t.Errorf("For %d cores, expected topology %ds/%dc/%dmax, got %ds/%dc/%dmax",
					test.desiredCores,
					test.sockets, test.cores, test.coresPerSocket,
					sockets, cores, coresPerSocket,
				)
			}
		})
	}
}

var _ = Describe("GetActivePodName", func() {
	It("should return the name of the active pod if it exists", func() {
		vm := &v1alpha2.VirtualMachine{
			Status: v1alpha2.VirtualMachineStatus{
				VirtualMachinePods: []v1alpha2.VirtualMachinePod{
					{
						Name:   "test-not-active",
						Active: false,
					},
					{
						Name:   "test-active",
						Active: true,
					},
				},
			},
		}

		podName, ok := GetActivePodName(vm)
		Expect(ok).To(BeTrue(), "must return pod name")
		Expect(podName).To(Equal("test-active"), "must return test-active pod name")
	})

	It("should not return pod name if no pod is active", func() {
		vm := &v1alpha2.VirtualMachine{
			Status: v1alpha2.VirtualMachineStatus{
				VirtualMachinePods: []v1alpha2.VirtualMachinePod{
					{
						Name:   "test-not-active",
						Active: false,
					},
					{
						Name:   "test-not-active-2",
						Active: false,
					},
				},
			},
		}

		podName, ok := GetActivePodName(vm)
		Expect(ok).To(BeFalse(), "must not return pod name")
		Expect(podName).To(Equal(""), "must return empty pod name")
	})
})

func newVMScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(v1alpha2.AddToScheme(scheme)).To(Succeed())
	Expect(virtv1.AddToScheme(scheme)).To(Succeed())
	return scheme
}

var _ = Describe("HasBlockDeviceStatusRef", func() {
	vmWithRefs := v1alpha2.VirtualMachine{
		Status: v1alpha2.VirtualMachineStatus{
			BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
				{Kind: v1alpha2.DiskDevice, Name: "disk-a"},
				{Kind: v1alpha2.ImageDevice, Name: "image-a"},
			},
		},
	}

	It("returns true when a matching kind and name are present", func() {
		Expect(HasBlockDeviceStatusRef(vmWithRefs, v1alpha2.DiskDevice, "disk-a")).To(BeTrue())
	})

	It("returns false when the name matches but the kind differs", func() {
		Expect(HasBlockDeviceStatusRef(vmWithRefs, v1alpha2.ImageDevice, "disk-a")).To(BeFalse())
	})

	It("returns false when the kind matches but the name differs", func() {
		Expect(HasBlockDeviceStatusRef(vmWithRefs, v1alpha2.DiskDevice, "disk-b")).To(BeFalse())
	})

	It("returns false when the VM has no block device refs", func() {
		Expect(HasBlockDeviceStatusRef(v1alpha2.VirtualMachine{}, v1alpha2.DiskDevice, "disk-a")).To(BeFalse())
	})
})

var _ = Describe("BlockDeviceUsage", func() {
	newVM := func(phase v1alpha2.MachinePhase) v1alpha2.VirtualMachine {
		return v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm", Namespace: "default"},
			Status: v1alpha2.VirtualMachineStatus{
				Phase: phase,
				BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
					{Kind: v1alpha2.DiskDevice, Name: "disk-a"},
				},
			},
		}
	}

	It("reports neither referenced nor mounted when the device is absent", func() {
		vm := newVM(v1alpha2.MachineRunning)
		vm.Status.BlockDeviceRefs = nil

		referenced, mounted, err := BlockDeviceUsage(context.Background(), nil, vm, v1alpha2.DiskDevice, "disk-a")
		Expect(err).NotTo(HaveOccurred())
		Expect(referenced).To(BeFalse())
		Expect(mounted).To(BeFalse())
	})

	It("reports referenced but not mounted when the phase is empty", func() {
		referenced, mounted, err := BlockDeviceUsage(context.Background(), nil, newVM(""), v1alpha2.DiskDevice, "disk-a")
		Expect(err).NotTo(HaveOccurred())
		Expect(referenced).To(BeTrue())
		Expect(mounted).To(BeFalse())
	})

	It("reports referenced and mounted for a running VM", func() {
		referenced, mounted, err := BlockDeviceUsage(context.Background(), nil, newVM(v1alpha2.MachineRunning), v1alpha2.DiskDevice, "disk-a")
		Expect(err).NotTo(HaveOccurred())
		Expect(referenced).To(BeTrue())
		Expect(mounted).To(BeTrue())
	})

	It("treats a stopped VM without a running launcher Pod as referenced but not mounted", func() {
		c := fake.NewClientBuilder().WithScheme(newVMScheme()).Build()

		referenced, mounted, err := BlockDeviceUsage(context.Background(), c, newVM(v1alpha2.MachineStopped), v1alpha2.DiskDevice, "disk-a")
		Expect(err).NotTo(HaveOccurred())
		Expect(referenced).To(BeTrue())
		Expect(mounted).To(BeFalse())
	})

	It("treats a stopped VM with a running launcher Pod as still mounted", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "virt-launcher-vm",
				Namespace: "default",
				Labels:    map[string]string{virtv1.VirtualMachineNameLabel: "vm"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}
		c := fake.NewClientBuilder().WithScheme(newVMScheme()).WithObjects(pod).Build()

		referenced, mounted, err := BlockDeviceUsage(context.Background(), c, newVM(v1alpha2.MachineStopped), v1alpha2.DiskDevice, "disk-a")
		Expect(err).NotTo(HaveOccurred())
		Expect(referenced).To(BeTrue())
		Expect(mounted).To(BeTrue())
	})
})

var _ = Describe("MountedVirtualMachineNames", func() {
	diskRef := v1alpha2.BlockDeviceStatusRef{Kind: v1alpha2.DiskDevice, Name: "disk-a"}

	newVM := func(namespace, name string, refs ...v1alpha2.BlockDeviceStatusRef) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Status: v1alpha2.VirtualMachineStatus{
				Phase:           v1alpha2.MachineRunning,
				BlockDeviceRefs: refs,
			},
		}
	}

	It("returns the sorted names of VMs that mount the device and skips the rest", func() {
		vmB := newVM("default", "vm-b", diskRef)
		vmA := newVM("default", "vm-a", diskRef)
		vmOther := newVM("default", "vm-other", v1alpha2.BlockDeviceStatusRef{Kind: v1alpha2.DiskDevice, Name: "disk-b"})
		c := fake.NewClientBuilder().WithScheme(newVMScheme()).WithObjects(vmB, vmA, vmOther).Build()

		names, err := MountedVirtualMachineNames(context.Background(), c, v1alpha2.DiskDevice, "disk-a", "default", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(names).To(Equal([]string{"vm-a", "vm-b"}))
	})

	It("prefixes each name with the namespace when withNamespace is true", func() {
		vm := newVM("ns1", "vm-a", diskRef)
		c := fake.NewClientBuilder().WithScheme(newVMScheme()).WithObjects(vm).Build()

		names, err := MountedVirtualMachineNames(context.Background(), c, v1alpha2.DiskDevice, "disk-a", "", true)
		Expect(err).NotTo(HaveOccurred())
		Expect(names).To(Equal([]string{"ns1/vm-a"}))
	})

	It("returns no names when no VM mounts the device", func() {
		vm := newVM("default", "vm-a", v1alpha2.BlockDeviceStatusRef{Kind: v1alpha2.DiskDevice, Name: "disk-b"})
		c := fake.NewClientBuilder().WithScheme(newVMScheme()).WithObjects(vm).Build()

		names, err := MountedVirtualMachineNames(context.Background(), c, v1alpha2.DiskDevice, "disk-a", "default", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(names).To(BeEmpty())
	})
})
