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

package indexer

import (
	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IndexPodByMigrationTargetNode indexes live migration target virt-launcher pods by the node they
// are bound to. A launcher keeps the migration label after the migration ends, so the index also
// holds pods of finished migrations: callers check the migration itself.
func IndexPodByMigrationTargetNode() (obj client.Object, field string, extractValue client.IndexerFunc) {
	return &corev1.Pod{}, IndexFieldPodByMigrationTargetNode, func(object client.Object) []string {
		pod, ok := object.(*corev1.Pod)
		if !ok || pod == nil || pod.Spec.NodeName == "" {
			return nil
		}
		if pod.Labels[virtv1.AppLabel] != "virt-launcher" || pod.Labels[virtv1.MigrationJobLabel] == "" {
			return nil
		}
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			return nil
		}
		return []string{pod.Spec.NodeName}
	}
}
