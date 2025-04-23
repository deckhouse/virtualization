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

var _ = Describe("TestStatisticHandler2", func() {
	const (
		vmName                = "vm"
		vmNamespace           = "default"
		podName               = "test-pod"
		nodeName              = "test-node"
		podUID      types.UID = "test-pod-uid"
	)

	newVM := func(cores int, coreFraction *string, memorySize string) *virtv2.VirtualMachine {
		vm := vmbuilder.New(
			vmbuilder.WithName(vmName),
			vmbuilder.WithNamespace(vmNamespace),
			vmbuilder.WithCPU(cores, coreFraction),
			vmbuilder.WithMemory(resource.MustParse(memorySize)),
		)
		vm.Status = virtv2.VirtualMachineStatus{
			Phase: virtv2.MachineRunning,
		}

		return vm
	}

	newKVVMI := func(requestCPU, limitCPU, requestMemory, limitMemory string) *virtv1.VirtualMachineInstance {
		kvvmi := newEmptyKVVMI(vmName, vmNamespace)
		kvvmi.Spec = virtv1.VirtualMachineInstanceSpec{
			Domain: virtv1.DomainSpec{
				Resources: virtv1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse(requestCPU),
						corev1.ResourceMemory: resource.MustParse(requestMemory),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse(limitCPU),
						corev1.ResourceMemory: resource.MustParse(limitMemory),
					},
				},
			},
		}
		kvvmi.Status = virtv1.VirtualMachineInstanceStatus{
			ActivePods: map[types.UID]string{podUID: podName},
			NodeName:   nodeName,
			Phase:      virtv1.Running,
		}
		return kvvmi
	}

	newPod := func(requestCPU, limitCPU, requestMemory, limitMemory string) *corev1.Pod {
		pod := newEmptyPOD(podName, vmNamespace, vmName)
		pod.UID = podUID
		pod.Spec = corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{
					Name: "compute",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(requestCPU),
							corev1.ResourceMemory: resource.MustParse(requestMemory),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(limitCPU),
							corev1.ResourceMemory: resource.MustParse(limitMemory),
						},
					},
				},
			},
		}
		return pod
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

	type expectedValues struct {
		CPUCores           int
		CPUCoreFraction    string
		CPURequestedCores  int64
		CPURuntimeOverhead int64

		TopologyCoresPerSocket int
		TopologySockets        int

		MemorySize            int64
		MemoryRuntimeOverhead int64
	}

	DescribeTable("Check Generated .status.resources",
		func(vm *virtv2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, pod *corev1.Pod, expect expectedValues) {
			fakeClient, vmResource, vmState = setupEnvironment(vm, kvvmi, pod)
			reconcile()

			newVM := &virtv2.VirtualMachine{}
			err := fakeClient.Get(ctx, client.ObjectKeyFromObject(vm), newVM)
			Expect(err).NotTo(HaveOccurred())

			res := newVM.Status.Resources
			Expect(res.CPU.Cores).To(Equal(expect.CPUCores))
			Expect(res.CPU.CoreFraction).To(Equal(expect.CPUCoreFraction))
			Expect(res.CPU.RequestedCores.MilliValue()).To(Equal(expect.CPURequestedCores))
			Expect(res.CPU.RuntimeOverhead.MilliValue()).To(Equal(expect.CPURuntimeOverhead))

			Expect(res.CPU.Topology).ShouldNot(BeNil())
			Expect(res.CPU.Topology.CoresPerSocket).To(Equal(expect.TopologyCoresPerSocket))
			Expect(res.CPU.Topology.Sockets).To(Equal(expect.TopologySockets))

			Expect(res.Memory.Size.Value()).To(Equal(expect.MemorySize))
			Expect(res.Memory.RuntimeOverhead.Value()).To(Equal(expect.MemoryRuntimeOverhead))
		},
		Entry("Case 1",
			newVM(1, ptr.To("50%"), "512Mi"),
			newKVVMI("500m", "1", "512Mi", "512Mi"),
			newPod("500m", "1", "755Mi", "755Mi"),
			expectedValues{
				CPUCores:           1,
				CPUCoreFraction:    "50%",
				CPURequestedCores:  500,
				CPURuntimeOverhead: 0,

				TopologyCoresPerSocket: 1,
				TopologySockets:        1,

				MemorySize:            536870912,
				MemoryRuntimeOverhead: 254803968,
			},
		),
	)
})
