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
	"errors"
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/watcher"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("LifeCycleHandler hotplug pod errors", func() {
	newContainerCreatingPod := func(vm *v1alpha2.VirtualMachine, name string, labels map[string]string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: vm.Namespace,
				Labels:    labels,
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason: "ContainerCreating",
							},
						},
					},
				},
			},
		}
	}
	newVolumeErrorEvent := func(vm *v1alpha2.VirtualMachine, podName string) *corev1.Event {
		return &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName + ".1",
				Namespace: vm.Namespace,
			},
			InvolvedObject: corev1.ObjectReference{
				Kind:      "Pod",
				Name:      podName,
				Namespace: vm.Namespace,
			},
			Type:          corev1.EventTypeWarning,
			Reason:        watcher.ReasonFailedMount,
			Message:       "unable to mount volume",
			LastTimestamp: metav1.NewTime(time.Now()),
		}
	}

	It("should return volume errors for launcher pod label", func() {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm",
				Namespace: "default",
			},
		}
		pod := newContainerCreatingPod(vm, "vm-pod", map[string]string{
			virtv1.VirtualMachineNameLabel: vm.Name,
		})
		event := newVolumeErrorEvent(vm, pod.Name)

		fakeClient, err := testutil.NewFakeClientWithObjects(vm, pod, event)
		Expect(err).NotTo(HaveOccurred())
		handler := NewLifeCycleHandler(fakeClient, nil)

		err = handler.checkVMPodVolumeErrors(context.Background(), vm, slog.Default())
		Expect(err).To(HaveOccurred())

		var volumeErr *VMPodVolumeError
		Expect(errors.As(err, &volumeErr)).To(BeTrue())
		Expect(volumeErr.Reason).To(Equal(watcher.ReasonFailedMount))
		Expect(volumeErr.Message).To(Equal("unable to mount volume"))
	})

	It("should return volume errors for hotplug pod resolved by kvvmi status", func() {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vm",
				Namespace: "default",
			},
		}
		hotplugPod := newContainerCreatingPod(vm, "hp-pod", nil)
		hotplugPod.UID = types.UID("hp-pod-uid")
		event := newVolumeErrorEvent(vm, hotplugPod.Name)
		kvvmi := &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vm.Name,
				Namespace: vm.Namespace,
			},
			Status: virtv1.VirtualMachineInstanceStatus{
				VolumeStatus: []virtv1.VolumeStatus{
					{
						Name: "vd-hotplug",
						HotplugVolume: &virtv1.HotplugVolumeStatus{
							AttachPodName: hotplugPod.Name,
							AttachPodUID:  hotplugPod.UID,
						},
					},
				},
			},
		}

		fakeClient, err := testutil.NewFakeClientWithObjects(vm, kvvmi, hotplugPod, event)
		Expect(err).NotTo(HaveOccurred())
		handler := NewLifeCycleHandler(fakeClient, nil)

		err = handler.checkVMPodVolumeErrors(context.Background(), vm, slog.Default())
		Expect(err).To(HaveOccurred())

		var volumeErr *VMPodVolumeError
		Expect(errors.As(err, &volumeErr)).To(BeTrue())
		Expect(volumeErr.Reason).To(Equal(watcher.ReasonFailedMount))
		Expect(volumeErr.Message).To(Equal("unable to mount volume"))
	})
})
