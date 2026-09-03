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
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
)

// Unschedulable fails the spec when a pod of the namespace is rejected by the
// scheduler for reasons other than cluster capacity: selector/affinity
// mismatches, untolerated taints, volume node affinity conflicts and the like
// point at a spec that can never make progress.
func Unschedulable(pods Client[*corev1.Pod], namespace string) FailFast {
	return New("pod "+namespace+"/", pods, func(pod *corev1.Pod) *Finding {
		if pod.DeletionTimestamp != nil {
			return nil
		}
		scheduled, found := conditions.GetPodCondition(corev1.PodScheduled, pod.Status.Conditions)
		if !found || scheduled.Status != corev1.ConditionFalse || scheduled.Reason != corev1.PodReasonUnschedulable {
			return nil
		}
		if unschedulableHealsItself(scheduled.Message) {
			return nil
		}
		return &Finding{
			Message: "is Unschedulable and the reason is not a resource shortage: " + scheduled.Message,
			Grace:   defaultGrace,
		}
	})
}

// unschedulableHealsItself reports the scheduler's rejection resolves without
// the spec doing anything: cluster capacity frees up as parallel specs finish,
// and volume binding completes once the PVC is provisioned.
func unschedulableHealsItself(message string) bool {
	for _, marker := range []string{
		// Insufficient cpu / memory / devices.kubevirt.io/kvm / ...
		"Insufficient",
		// The node's pod-count capacity is exhausted.
		"Too many pods",
		// "pod has unbound immediate PersistentVolumeClaims"
		"PersistentVolumeClaims",
		// The PreBind VolumeBinding plugin timed out while the PVC was still
		// provisioning; the next scheduling cycle retries the binding.
		"binding volumes",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
