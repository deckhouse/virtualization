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
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	kvv1 "kubevirt.io/api/core/v1"

	vmutil "github.com/deckhouse/virtualization-controller/pkg/common/vm"
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
func ShutdownReason(kvvmi *kvv1.VirtualMachineInstance, kvPods *corev1.PodList) ShutdownInfo {
	if kvvmi == nil || kvvmi.Status.Phase != kvv1.Succeeded {
		return ShutdownInfo{}
	}
	if kvPods == nil || len(kvPods.Items) == 0 {
		return ShutdownInfo{}
	}

	// Sort Pods in descending order to operate on the most recent Pod.
	sort.SliceStable(kvPods.Items, func(i, j int) bool {
		return kvPods.Items[i].CreationTimestamp.Compare(kvPods.Items[j].CreationTimestamp.Time) > 0
	})
	recentPod := kvPods.Items[0]
	// Power events are not available in Running state, only Completed Pod has termination message.
	if recentPod.Status.Phase != corev1.PodSucceeded {
		return ShutdownInfo{}
	}

	// Extract termination message from the container with VM.
	for _, contStatus := range recentPod.Status.ContainerStatuses {
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
			return ShutdownInfo{PodCompleted: true, Reason: GuestResetReason, Pod: recentPod}
		}
		if strings.Contains(msg, string(GuestShutdownReason)) {
			return ShutdownInfo{PodCompleted: true, Reason: GuestShutdownReason, Pod: recentPod}
		}
	}

	return ShutdownInfo{PodCompleted: true, Pod: recentPod}
}

type ShutdownInfo struct {
	Reason       GuestSignalReason
	PodCompleted bool
	Pod          corev1.Pod
}
