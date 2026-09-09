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

// NodeNotReady fails the spec when a cluster node loses its Ready condition:
// specs with workloads on that node hang silently, so it is better to fail
// them at once and keep the failure attributable.
func NodeNotReady(nodes Client[*corev1.Node]) FailFast {
	return New("node ", nodes, func(node *corev1.Node) *Finding {
		for _, cond := range node.Status.Conditions {
			if cond.Type != corev1.NodeReady {
				continue
			}
			if cond.Status == corev1.ConditionTrue {
				return nil
			}
			return &Finding{
				Message: fmt.Sprintf("is not Ready (%s): %s", cond.Reason, cond.Message),
				Grace:   defaultGrace,
			}
		}
		return nil
	})
}
