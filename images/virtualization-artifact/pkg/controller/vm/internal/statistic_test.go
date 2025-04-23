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

package internal

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("TestStatisticHandler", func() {
	const (
		vmName                = "vm"
		vmNamespace           = "default"
		podName               = "test-pod"
		nodeName              = "test-node"
		podUID      types.UID = "test-pod-uid"
	)
	createPod := func() *corev1.Pod {
		pod := newEmptyPOD(podName, vmNamespace, vmName)
		pod.UID = podUID
		pod.Spec = corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{
					Name: "compute",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("755Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("755Mi"),
						},
					},
				},
			},
		}
		return pod
	}

	createVM := func(phase virtv2.MachinePhase, stats *virtv2.VirtualMachineStats, guestOSInfo virtv1.VirtualMachineInstanceGuestOSInfo) *virtv2.VirtualMachine {
		vm := vmbuilder.New(
			vmbuilder.WithName(vmName),
			vmbuilder.WithNamespace(vmNamespace),
			vmbuilder.WithCPU(1, ptr.To("50%")),
			vmbuilder.WithMemory(resource.MustParse("512Mi")),
		)
		vm.Status = virtv2.VirtualMachineStatus{
			Phase:       phase,
			Stats:       stats,
			GuestOSInfo: guestOSInfo,
		}

		return vm
	}

	createKVVMI := func(phase virtv1.VirtualMachineInstancePhase, guestOSInfo virtv1.VirtualMachineInstanceGuestOSInfo) *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)
		kvvmi.Spec = virtv1.VirtualMachineInstanceSpec{
			Domain: virtv1.DomainSpec{
				Resources: virtv1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
			},
		}
		kvvmi.Status = virtv1.VirtualMachineInstanceStatus{
			ActivePods:  map[types.UID]string{podUID: podName},
			NodeName:    nodeName,
			Phase:       phase,
			GuestOSInfo: guestOSInfo,
		}
		return kvvmi
	}

	var (
		ctx        = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient client.Client
		vmResource *reconciler.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus]
		vmState    state.VirtualMachineState
	)

	reconcile := func() {
		h := NewStatisticHandler(fakeClient)
		_, err := h.Handle(context.Background(), vmState)
		Expect(err).NotTo(HaveOccurred())
		err = vmResource.Update(ctx)
		Expect(err).NotTo(HaveOccurred())
	}

	AfterEach(func() {
		fakeClient = nil
		vmResource = nil
		vmState = nil
	})

	It("Check Generated .status.resources", func() {
		vm := createVM(virtv2.MachineRunning, nil, virtv1.VirtualMachineInstanceGuestOSInfo{})
		kvvmi := createKVVMI(virtv1.Running, virtv1.VirtualMachineInstanceGuestOSInfo{Name: "test"})
		pod := createPod()

		fakeClient, vmResource, vmState = setupEnvironment(vm, kvvmi, pod)
		reconcile()

		newVM := &virtv2.VirtualMachine{}
		err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
		Expect(err).NotTo(HaveOccurred())

		res := newVM.Status.Resources
		Expect(res.CPU.Cores).To(Equal(1))
		Expect(res.CPU.CoreFraction).To(Equal("50%"))
		Expect(res.CPU.RequestedCores.MilliValue()).To(Equal(int64(500)))
		Expect(res.CPU.RuntimeOverhead.MilliValue()).To(Equal(int64(0)))

		Expect(res.CPU.Topology).ShouldNot(BeNil())
		Expect(res.CPU.Topology.CoresPerSocket).To(Equal(1))
		Expect(res.CPU.Topology.Sockets).To(Equal(1))

		Expect(res.Memory.Size.Value()).To(Equal(int64(536870912)))
		Expect(res.Memory.RuntimeOverhead.Value()).To(Equal(int64(254803968)))
	})
})
