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

package kvvm

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
)

var _ = Describe("GetVMPod", func() {
	const (
		podName  = "virt-launcher-test"
		nodeName = "worker-1"
	)

	var (
		podUID types.UID = "pod-uid"
		kvvmi  *virtv1.VirtualMachineInstance
		pod    corev1.Pod
	)

	BeforeEach(func() {
		kvvmi = &virtv1.VirtualMachineInstance{
			Status: virtv1.VirtualMachineInstanceStatus{
				Phase:    virtv1.Running,
				NodeName: nodeName,
				ActivePods: map[types.UID]string{
					podUID: podName,
				},
			},
		}
		pod = corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: podName,
				UID:  podUID,
			},
			Spec: corev1.PodSpec{
				NodeName: nodeName,
			},
		}
	})

	It("returns pod when node names match", func() {
		result := GetVMPod(kvvmi, &corev1.PodList{Items: []corev1.Pod{pod}})
		Expect(result).NotTo(BeNil())
		Expect(result.Name).To(Equal(podName))
	})

	It("skips pod on different node when kvvmi is running", func() {
		pod.Spec.NodeName = "worker-2"

		result := GetVMPod(kvvmi, &corev1.PodList{Items: []corev1.Pod{pod}})
		Expect(result).To(BeNil())
	})

	It("returns pod when kvvmi is completed and nodeName is empty", func() {
		kvvmi.Status.Phase = virtv1.Failed
		kvvmi.Status.NodeName = ""
		pod.Spec.NodeName = "worker-2"

		result := GetVMPod(kvvmi, &corev1.PodList{Items: []corev1.Pod{pod}})
		Expect(result).NotTo(BeNil())
		Expect(result.Name).To(Equal(podName))
	})

	It("returns pod when kvvmi is succeeded and nodeName is empty", func() {
		kvvmi.Status.Phase = virtv1.Succeeded
		kvvmi.Status.NodeName = ""
		pod.Spec.NodeName = "worker-2"

		result := GetVMPod(kvvmi, &corev1.PodList{Items: []corev1.Pod{pod}})
		Expect(result).NotTo(BeNil())
		Expect(result.Name).To(Equal(podName))
	})

	It("skips pod on different node when kvvmi is running with empty nodeName", func() {
		kvvmi.Status.NodeName = ""
		pod.Spec.NodeName = "worker-2"

		result := GetVMPod(kvvmi, &corev1.PodList{Items: []corev1.Pod{pod}})
		Expect(result).To(BeNil())
	})
})
