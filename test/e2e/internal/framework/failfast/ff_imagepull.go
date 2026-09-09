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

// ImagePull fails the spec when a container of a namespace pod cannot pull
// its image. A malformed reference never heals and fails immediately; a pull
// error gets a grace period to survive registry blips and the kubelet's
// backoff.
func ImagePull(pods Client[*corev1.Pod], namespace string) FailFast {
	return New("pod "+namespace+"/", pods, func(pod *corev1.Pod) *Finding {
		if pod.DeletionTimestamp != nil {
			return nil
		}
		for _, status := range allContainerStatuses(pod) {
			waiting := status.State.Waiting
			if waiting == nil {
				continue
			}
			switch waiting.Reason {
			case "InvalidImageName", "ErrImageNeverPull":
				return &Finding{
					Message: fmt.Sprintf("container %q can never pull its image (%s): %s",
						status.Name, waiting.Reason, waiting.Message),
				}
			case "ErrImagePull", "ImagePullBackOff":
				return &Finding{
					Message: fmt.Sprintf("container %q cannot pull its image (%s): %s",
						status.Name, waiting.Reason, waiting.Message),
					Grace: defaultGrace,
				}
			}
		}
		return nil
	})
}

func allContainerStatuses(pod *corev1.Pod) []corev1.ContainerStatus {
	statuses := make([]corev1.ContainerStatus, 0, len(pod.Status.InitContainerStatuses)+len(pod.Status.ContainerStatuses))
	statuses = append(statuses, pod.Status.InitContainerStatuses...)
	return append(statuses, pod.Status.ContainerStatuses...)
}
