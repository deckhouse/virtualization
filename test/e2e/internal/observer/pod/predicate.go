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

package pod

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

func BeSucceeded() Predicate {
	return func(pod *corev1.Pod) (bool, error) {
		return pod.Status.Phase == corev1.PodSucceeded, nil
	}
}

func BeFailed() Predicate {
	return func(pod *corev1.Pod) (bool, error) {
		if pod.Status.Phase == corev1.PodFailed {
			return true, fmt.Errorf("pod entered Failed phase: %s", containerDiagnostics(pod))
		}
		return false, nil
	}
}

func containerDiagnostics(pod *corev1.Pod) string {
	parts := make([]string, 0, len(pod.Status.ContainerStatuses))
	for _, status := range pod.Status.ContainerStatuses {
		switch {
		case status.State.Waiting != nil:
			parts = append(parts, fmt.Sprintf("%s waiting %s: %s", status.Name, status.State.Waiting.Reason, status.State.Waiting.Message))
		case status.State.Terminated != nil:
			parts = append(parts, fmt.Sprintf("%s terminated %s(%d): %s", status.Name, status.State.Terminated.Reason, status.State.Terminated.ExitCode, status.State.Terminated.Message))
		}
	}
	if len(parts) == 0 {
		return "no container diagnostics"
	}
	return strings.Join(parts, "; ")
}
