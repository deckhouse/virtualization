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

package powerstate

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"

	vmutil "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type GuestSignalReason string

const (
	// GuestResetReason - a reboot command was issued from inside the VM.
	GuestResetReason GuestSignalReason = "guest-reset"

	// GuestShutdownReason - a poweroff command was issued from inside the VM.
	GuestShutdownReason GuestSignalReason = "guest-shutdown"
)

// ShutdownReason returns a shutdown reason from the Completed Pod with VM:
// - guest-reset — reboot was issued inside the VM
// - guest-shutdown — poweroff was issued inside the VM
// - empty string means VM is still Running or was exited without event.
// Shutdown termination message
// {"event":"SHUTDOWN","details":"{\"guest\":true,\"reason\":\"guest-shutdown\"}"}
// {"event":"SHUTDOWN","details":"{\"guest\":false,\"reason\":\"host-signal\"}"}
// Reset termination message
// {"event":"SHUTDOWN","details":"{\"guest\":true,\"reason\":\"guest-reset\"}"}
// {"event":"SHUTDOWN","details":"{\"guest\":false,\"reason\":\"host-signal\"}"}
func ShutdownReason(vm *v1alpha2.VirtualMachine, kvvmi *virtv1.VirtualMachineInstance, kvPods *corev1.PodList) ShutdownInfo {
	if kvvmi == nil || kvvmi.Status.Phase != virtv1.Succeeded {
		return ShutdownInfo{}
	}
	if kvPods == nil || len(kvPods.Items) == 0 {
		return ShutdownInfo{}
	}
	activePod, ok := getActivePod(vm, kvPods)
	if !ok {
		return ShutdownInfo{}
	}

	// Power events are not available in Running state, only Completed Pod has termination message.
	if activePod.Status.Phase != corev1.PodSucceeded {
		return ShutdownInfo{}
	}

	// Extract termination message from the container with VM.
	for _, contStatus := range activePod.Status.ContainerStatuses {
		if !vmutil.IsComputeContainer(contStatus.Name) {
			continue
		}
		msg := ""
		if contStatus.LastTerminationState.Terminated != nil {
			msg = contStatus.LastTerminationState.Terminated.Message
		}
		if contStatus.State.Terminated != nil {
			msg = contStatus.State.Terminated.Message
		}
		if strings.Contains(msg, string(GuestResetReason)) {
			return ShutdownInfo{PodCompleted: true, Reason: GuestResetReason, Pod: activePod}
		}
		if strings.Contains(msg, string(GuestShutdownReason)) {
			return ShutdownInfo{PodCompleted: true, Reason: GuestShutdownReason, Pod: activePod}
		}
	}

	return ShutdownInfo{PodCompleted: true, Pod: activePod}
}

func getActivePod(vm *v1alpha2.VirtualMachine, kvPods *corev1.PodList) (corev1.Pod, bool) {
	activePodName, ok := vmutil.GetActivePodName(vm)
	if !ok {
		return corev1.Pod{}, false
	}

	for _, pod := range kvPods.Items {
		if pod.Name == activePodName {
			return pod, true
		}
	}

	return corev1.Pod{}, false
}

type ShutdownInfo struct {
	Reason       GuestSignalReason
	PodCompleted bool
	Pod          corev1.Pod
}
