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
)

const (
	// DefaultVMContainerName - a container name with virt-launcher, libvirt and qemu processes.
	DefaultVMContainerName = "compute"

	// GuestResetReason - a reboot command was issued from inside the VM.
	GuestResetReason = "guest-reset"

	// GuestShutdownReason - a poweroff command was issued from inside the VM.
	GuestShutdownReason = "guest-shutdown"
)

// ShutdownReason returns a shutdown reason from the Completed Pod with VM:
// - guest-reset — reboot was issued inside the VM
// - guest-shutdown — poweroff was issued inside the VM
// - empty string means VM is still Running or was exited without event.
func ShutdownReason(kvvmi *kvv1.VirtualMachineInstance, kvPods *corev1.PodList) (bool, string) {
	if kvvmi == nil || kvvmi.Status.Phase != kvv1.Succeeded {
		return false, ""
	}
	if kvPods == nil || len(kvPods.Items) == 0 {
		return false, ""
	}

	// Sort Pods in descending order to operate on the most recent Pod.
	sort.SliceStable(kvPods.Items, func(i, j int) bool {
		return kvPods.Items[i].CreationTimestamp.Compare(kvPods.Items[j].CreationTimestamp.Time) > 0
	})
	recentPod := kvPods.Items[0]
	// Power events are not available in Running state, only Completed Pod has termination message.
	if recentPod.Status.Phase != corev1.PodSucceeded {
		return false, ""
	}

	// Extract termination mesage from the "compute" container.
	for _, contStatus := range recentPod.Status.ContainerStatuses {
		// "compute" is a default container name for VM Pod.
		if contStatus.Name != DefaultVMContainerName {
			continue
		}
		msg := ""
		if contStatus.LastTerminationState.Terminated != nil {
			msg = contStatus.LastTerminationState.Terminated.Message
		}
		if contStatus.State.Terminated != nil {
			msg = contStatus.State.Terminated.Message
		}
		if strings.Contains(msg, GuestResetReason) {
			return true, GuestResetReason
		}
		if strings.Contains(msg, GuestShutdownReason) {
			return true, GuestShutdownReason
		}
	}

	return true, ""
}
