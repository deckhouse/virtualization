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

package resourcequota

import (
	corev1 "k8s.io/api/core/v1"
)

// BeEnforced reports that the ResourceQuota status reflects the hard limits
// from spec, so admission-time enforcement is in effect.
func BeEnforced() Predicate {
	return func(rq *corev1.ResourceQuota) (bool, error) {
		pods, ok := rq.Status.Hard[corev1.ResourceName("count/pods")]
		if !ok || pods.Sign() != 0 {
			return false, nil
		}
		pvcs, ok := rq.Status.Hard[corev1.ResourceName("count/persistentvolumeclaims")]
		if !ok || pvcs.Sign() != 0 {
			return false, nil
		}
		return true, nil
	}
}
