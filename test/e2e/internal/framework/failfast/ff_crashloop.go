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

package failfast

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// crashLoopRestartThreshold is how many restarts a container in
// CrashLoopBackOff must accumulate before the spec is failed: the first
// couple of restarts may still be a transient (a dependency that came up
// late), three failures in a row are a pattern.
const crashLoopRestartThreshold = 3

// CrashLoop fails the spec when a container of a namespace pod is currently
// in CrashLoopBackOff with enough restarts behind it to rule out a transient.
// A repeatedly OOMKilled container is reported separately: the fix (more
// memory) differs from an ordinary crash.
func CrashLoop(pods Client[*corev1.Pod], namespace string) FailFast {
	return New("pod "+namespace+"/", pods, func(pod *corev1.Pod) *Finding {
		if pod.DeletionTimestamp != nil {
			return nil
		}
		for _, status := range allContainerStatuses(pod) {
			waiting := status.State.Waiting
			if waiting == nil || waiting.Reason != "CrashLoopBackOff" || status.RestartCount < crashLoopRestartThreshold {
				continue
			}
			if last := status.LastTerminationState.Terminated; last != nil {
				if last.Reason == "OOMKilled" {
					return &Finding{
						Message: fmt.Sprintf("container %q is repeatedly OOMKilled (%d restarts)",
							status.Name, status.RestartCount),
					}
				}
				return &Finding{
					Message: fmt.Sprintf("container %q is in CrashLoopBackOff (%d restarts), last exit: %s(%d): %s",
						status.Name, status.RestartCount, last.Reason, last.ExitCode, last.Message),
				}
			}
			return &Finding{
				Message: fmt.Sprintf("container %q is in CrashLoopBackOff (%d restarts)",
					status.Name, status.RestartCount),
			}
		}
		return nil
	})
}
