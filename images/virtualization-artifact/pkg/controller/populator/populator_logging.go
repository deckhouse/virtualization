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

package populator

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
)

type importerPodSnapshot struct {
	role  string
	name  string
	phase corev1.PodPhase
}

func (r *Reconciler) logImporterPodTransitions(
	pvc *corev1.PersistentVolumeClaim,
	strategy string,
	before, after []importerPodSnapshot,
) {
	afterByRole := make(map[string]importerPodSnapshot, len(after))
	for _, snap := range after {
		afterByRole[snap.role] = snap
	}

	for _, prev := range before {
		next, ok := afterByRole[prev.role]
		if !ok {
			continue
		}
		if prev.phase == "" && next.phase != "" {
			r.log.Info("PVC population importer pod created",
				"namespace", pvc.Namespace,
				"pvc", pvc.Name,
				"strategy", strategy,
				"podRole", next.role,
				"pod", next.name,
				"phase", string(next.phase),
			)
		}
		if prev.phase != "" && prev.phase != corev1.PodSucceeded && next.phase == corev1.PodSucceeded {
			r.log.Info("PVC population importer pod finished",
				"namespace", pvc.Namespace,
				"pvc", pvc.Name,
				"strategy", strategy,
				"podRole", next.role,
				"pod", next.name,
			)
		}
	}
}

func (r *Reconciler) rebindPending(ctx context.Context, pvc *corev1.PersistentVolumeClaim, sup supplements.Generator, strategy string) bool {
	if pvc.Annotations[annotations.AnnPVCPopulationDone] == "true" {
		return false
	}

	snapshots, err := r.importerPodSnapshots(ctx, sup, strategy)
	if err != nil {
		return false
	}

	switch strategy {
	case service.PopulationStrategyHostAssigned:
		for _, snap := range snapshots {
			if snap.role == "target-importer" && snap.phase == corev1.PodSucceeded {
				return true
			}
		}
	case service.PopulationStrategyDVCR:
		for _, snap := range snapshots {
			if snap.role == "importer" && snap.phase == corev1.PodSucceeded {
				return true
			}
		}
	}
	return false
}

func (r *Reconciler) importerPodSnapshots(ctx context.Context, sup supplements.Generator, strategy string) ([]importerPodSnapshot, error) {
	var keys []struct {
		role string
		key  types.NamespacedName
	}

	switch strategy {
	case service.PopulationStrategyHostAssigned:
		keys = append(keys,
			struct {
				role string
				key  types.NamespacedName
			}{"source-importer", sup.PVCSourceImporterPod()},
			struct {
				role string
				key  types.NamespacedName
			}{"target-importer", sup.PVCTargetImporterPod()},
		)
	case service.PopulationStrategyDVCR:
		keys = append(keys, struct {
			role string
			key  types.NamespacedName
		}{"importer", sup.PVCImporterPod()})
	}

	snapshots := make([]importerPodSnapshot, 0, len(keys))
	for _, item := range keys {
		pod, err := object.FetchObject(ctx, item.key, r.client, &corev1.Pod{})
		if err != nil {
			return nil, fmt.Errorf("fetch %s pod: %w", item.role, err)
		}
		snap := importerPodSnapshot{role: item.role, name: item.key.Name}
		if pod != nil {
			snap.phase = pod.Status.Phase
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, nil
}
