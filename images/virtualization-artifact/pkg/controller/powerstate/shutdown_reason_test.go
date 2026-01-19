/*
Copyright 2025 Flant JSC

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

package powerstate

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("ShutdownReason", func() {
	var (
		vm    *v1alpha2.VirtualMachine
		kvvmi *virtv1.VirtualMachineInstance
		pods  *corev1.PodList
	)

	BeforeEach(func() {
		vm = &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vm",
				Namespace: "test-namespace",
			},
		}
	})

	Context("when kvvmi is nil", func() {
		It("should return empty ShutdownInfo", func() {
			result := ShutdownReason(vm, nil, pods)
			Expect(result).To(Equal(ShutdownInfo{}))
		})
	})

	Context("when kvvmi is not in Succeeded phase", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Running,
				},
			}
		})

		It("should return empty ShutdownInfo", func() {
			result := ShutdownReason(vm, kvvmi, pods)
			Expect(result).To(Equal(ShutdownInfo{}))
		})
	})

	Context("when kvPods is nil", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Succeeded,
				},
			}
		})

		It("should return empty ShutdownInfo", func() {
			result := ShutdownReason(vm, kvvmi, nil)
			Expect(result).To(Equal(ShutdownInfo{}))
		})
	})

	Context("when kvPods is empty", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Succeeded,
				},
			}
			pods = &corev1.PodList{
				Items: []corev1.Pod{},
			}
		})

		It("should return empty ShutdownInfo", func() {
			result := ShutdownReason(vm, kvvmi, pods)
			Expect(result).To(Equal(ShutdownInfo{}))
		})
	})

	Context("when VM has no active pod", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Succeeded,
				},
			}
			vm.Status = v1alpha2.VirtualMachineStatus{
				VirtualMachinePods: []v1alpha2.VirtualMachinePod{
					{
						Name:   "test-pod",
						Active: false,
					},
				},
			}
			pods = &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "test-pod",
							Namespace:         "test-namespace",
							CreationTimestamp: metav1.Now(),
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodSucceeded,
						},
					},
				},
			}
		})

		It("should return empty ShutdownInfo", func() {
			result := ShutdownReason(vm, kvvmi, pods)
			Expect(result).To(Equal(ShutdownInfo{}))
		})
	})

	Context("when active pod is not found in pod list", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Succeeded,
				},
			}
			vm.Status = v1alpha2.VirtualMachineStatus{
				VirtualMachinePods: []v1alpha2.VirtualMachinePod{
					{
						Name:   "active-pod",
						Active: true,
					},
				},
			}
			pods = &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "other-pod",
							Namespace:         "test-namespace",
							CreationTimestamp: metav1.Now(),
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodSucceeded,
						},
					},
				},
			}
		})

		It("should return empty ShutdownInfo", func() {
			result := ShutdownReason(vm, kvvmi, pods)
			Expect(result).To(Equal(ShutdownInfo{}))
		})
	})

	Context("when pod is not in Succeeded phase", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Succeeded,
				},
			}
			vm.Status = v1alpha2.VirtualMachineStatus{
				VirtualMachinePods: []v1alpha2.VirtualMachinePod{
					{
						Name:   "test-pod",
						Active: true,
					},
				},
			}
			pods = &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "test-pod",
							Namespace:         "test-namespace",
							CreationTimestamp: metav1.Now(),
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
				},
			}
		})

		It("should return empty ShutdownInfo", func() {
			result := ShutdownReason(vm, kvvmi, pods)
			Expect(result).To(Equal(ShutdownInfo{}))
		})
	})

	Context("when pod has guest-shutdown reason in State.Terminated", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Succeeded,
				},
			}
			vm.Status = v1alpha2.VirtualMachineStatus{
				VirtualMachinePods: []v1alpha2.VirtualMachinePod{
					{
						Name:   "test-pod",
						Active: true,
					},
				},
			}
			pods = &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "test-pod",
							Namespace:         "test-namespace",
							CreationTimestamp: metav1.Now(),
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodSucceeded,
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "test-compute",
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											Message: `{"event":"SHUTDOWN","details":"{\"guest\":true,\"reason\":\"guest-shutdown\"}"}`,
										},
									},
								},
							},
						},
					},
				},
			}
		})

		It("should return ShutdownInfo with GuestShutdownReason", func() {
			result := ShutdownReason(vm, kvvmi, pods)
			Expect(result.PodCompleted).To(BeTrue())
			Expect(result.Reason).To(Equal(GuestShutdownReason))
			Expect(result.Pod.Name).To(Equal("test-pod"))
		})
	})

	Context("when pod has guest-reset reason in State.Terminated", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Succeeded,
				},
			}
			vm.Status = v1alpha2.VirtualMachineStatus{
				VirtualMachinePods: []v1alpha2.VirtualMachinePod{
					{
						Name:   "test-pod",
						Active: true,
					},
				},
			}
			pods = &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "test-pod",
							Namespace:         "test-namespace",
							CreationTimestamp: metav1.Now(),
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodSucceeded,
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "test-compute",
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											Message: `{"event":"SHUTDOWN","details":"{\"guest\":true,\"reason\":\"guest-reset\"}"}`,
										},
									},
								},
							},
						},
					},
				},
			}
		})

		It("should return ShutdownInfo with GuestResetReason", func() {
			result := ShutdownReason(vm, kvvmi, pods)
			Expect(result.PodCompleted).To(BeTrue())
			Expect(result.Reason).To(Equal(GuestResetReason))
			Expect(result.Pod.Name).To(Equal("test-pod"))
		})
	})

	Context("when pod has guest-shutdown reason in LastTerminationState.Terminated", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Succeeded,
				},
			}
			vm.Status = v1alpha2.VirtualMachineStatus{
				VirtualMachinePods: []v1alpha2.VirtualMachinePod{
					{
						Name:   "test-pod",
						Active: true,
					},
				},
			}
			pods = &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "test-pod",
							Namespace:         "test-namespace",
							CreationTimestamp: metav1.Now(),
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodSucceeded,
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "test-compute",
									LastTerminationState: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											Message: `{"event":"SHUTDOWN","details":"{\"guest\":true,\"reason\":\"guest-shutdown\"}"}`,
										},
									},
								},
							},
						},
					},
				},
			}
		})

		It("should return ShutdownInfo with GuestShutdownReason", func() {
			result := ShutdownReason(vm, kvvmi, pods)
			Expect(result.PodCompleted).To(BeTrue())
			Expect(result.Reason).To(Equal(GuestShutdownReason))
			Expect(result.Pod.Name).To(Equal("test-pod"))
		})
	})

	Context("when pod has guest-reset reason in LastTerminationState.Terminated", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Succeeded,
				},
			}
			vm.Status = v1alpha2.VirtualMachineStatus{
				VirtualMachinePods: []v1alpha2.VirtualMachinePod{
					{
						Name:   "test-pod",
						Active: true,
					},
				},
			}
			pods = &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "test-pod",
							Namespace:         "test-namespace",
							CreationTimestamp: metav1.Now(),
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodSucceeded,
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "test-compute",
									LastTerminationState: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											Message: `{"event":"SHUTDOWN","details":"{\"guest\":true,\"reason\":\"guest-reset\"}"}`,
										},
									},
								},
							},
						},
					},
				},
			}
		})

		It("should return ShutdownInfo with GuestResetReason", func() {
			result := ShutdownReason(vm, kvvmi, pods)
			Expect(result.PodCompleted).To(BeTrue())
			Expect(result.Reason).To(Equal(GuestResetReason))
			Expect(result.Pod.Name).To(Equal("test-pod"))
		})
	})

	Context("when pod has host-signal reason (not guest)", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Succeeded,
				},
			}
			vm.Status = v1alpha2.VirtualMachineStatus{
				VirtualMachinePods: []v1alpha2.VirtualMachinePod{
					{
						Name:   "test-pod",
						Active: true,
					},
				},
			}
			pods = &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "test-pod",
							Namespace:         "test-namespace",
							CreationTimestamp: metav1.Now(),
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodSucceeded,
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "test-compute",
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											Message: `{"event":"SHUTDOWN","details":"{\"guest\":false,\"reason\":\"host-signal\"}"}`,
										},
									},
								},
							},
						},
					},
				},
			}
		})

		It("should return ShutdownInfo without reason", func() {
			result := ShutdownReason(vm, kvvmi, pods)
			Expect(result.PodCompleted).To(BeTrue())
			Expect(result.Reason).To(BeEmpty())
			Expect(result.Pod.Name).To(Equal("test-pod"))
		})
	})

	Context("when pod has no termination message", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Succeeded,
				},
			}
			vm.Status = v1alpha2.VirtualMachineStatus{
				VirtualMachinePods: []v1alpha2.VirtualMachinePod{
					{
						Name:   "test-pod",
						Active: true,
					},
				},
			}
			pods = &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "test-pod",
							Namespace:         "test-namespace",
							CreationTimestamp: metav1.Now(),
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodSucceeded,
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "test-compute",
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{},
									},
								},
							},
						},
					},
				},
			}
		})

		It("should return ShutdownInfo without reason", func() {
			result := ShutdownReason(vm, kvvmi, pods)
			Expect(result.PodCompleted).To(BeTrue())
			Expect(result.Reason).To(BeEmpty())
			Expect(result.Pod.Name).To(Equal("test-pod"))
		})
	})

	Context("when pod has no compute container", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Succeeded,
				},
			}
			vm.Status = v1alpha2.VirtualMachineStatus{
				VirtualMachinePods: []v1alpha2.VirtualMachinePod{
					{
						Name:   "test-pod",
						Active: true,
					},
				},
			}
			pods = &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "test-pod",
							Namespace:         "test-namespace",
							CreationTimestamp: metav1.Now(),
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodSucceeded,
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "other-container",
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											Message: `{"event":"SHUTDOWN","details":"{\"guest\":true,\"reason\":\"guest-shutdown\"}"}`,
										},
									},
								},
							},
						},
					},
				},
			}
		})

		It("should return ShutdownInfo without reason", func() {
			result := ShutdownReason(vm, kvvmi, pods)
			Expect(result.PodCompleted).To(BeTrue())
			Expect(result.Reason).To(BeEmpty())
			Expect(result.Pod.Name).To(Equal("test-pod"))
		})
	})

	Context("when there are multiple pods", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Succeeded,
				},
			}
			vm.Status = v1alpha2.VirtualMachineStatus{
				VirtualMachinePods: []v1alpha2.VirtualMachinePod{
					{
						Name:   "older-pod",
						Active: false,
					},
					{
						Name:   "active-pod",
						Active: true,
					},
				},
			}
			now := metav1.Now()
			olderTime := metav1.NewTime(now.AddDate(0, 0, -1))
			pods = &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "older-pod",
							Namespace:         "test-namespace",
							CreationTimestamp: olderTime,
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodSucceeded,
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "test-compute",
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											Message: `{"event":"SHUTDOWN","details":"{\"guest\":true,\"reason\":\"guest-shutdown\"}"}`,
										},
									},
								},
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "active-pod",
							Namespace:         "test-namespace",
							CreationTimestamp: now,
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodSucceeded,
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "test-compute",
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											Message: `{"event":"SHUTDOWN","details":"{\"guest\":true,\"reason\":\"guest-reset\"}"}`,
										},
									},
								},
							},
						},
					},
				},
			}
		})

		It("should use the active pod from VM status", func() {
			result := ShutdownReason(vm, kvvmi, pods)
			Expect(result.PodCompleted).To(BeTrue())
			Expect(result.Reason).To(Equal(GuestResetReason))
			Expect(result.Pod.Name).To(Equal("active-pod"))
		})
	})

	Context("when State.Terminated takes precedence over LastTerminationState", func() {
		BeforeEach(func() {
			kvvmi = &virtv1.VirtualMachineInstance{
				Status: virtv1.VirtualMachineInstanceStatus{
					Phase: virtv1.Succeeded,
				},
			}
			vm.Status = v1alpha2.VirtualMachineStatus{
				VirtualMachinePods: []v1alpha2.VirtualMachinePod{
					{
						Name:   "test-pod",
						Active: true,
					},
				},
			}
			pods = &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "test-pod",
							Namespace:         "test-namespace",
							CreationTimestamp: metav1.Now(),
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodSucceeded,
							ContainerStatuses: []corev1.ContainerStatus{
								{
									Name: "test-compute",
									State: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											Message: `{"event":"SHUTDOWN","details":"{\"guest\":true,\"reason\":\"guest-shutdown\"}"}`,
										},
									},
									LastTerminationState: corev1.ContainerState{
										Terminated: &corev1.ContainerStateTerminated{
											Message: `{"event":"SHUTDOWN","details":"{\"guest\":true,\"reason\":\"guest-reset\"}"}`,
										},
									},
								},
							},
						},
					},
				},
			}
		})

		It("should use State.Terminated message", func() {
			result := ShutdownReason(vm, kvvmi, pods)
			Expect(result.PodCompleted).To(BeTrue())
			Expect(result.Reason).To(Equal(GuestShutdownReason))
			Expect(result.Pod.Name).To(Equal("test-pod"))
		})
	})
})
