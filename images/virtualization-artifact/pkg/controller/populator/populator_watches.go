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

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
)

func addWatches(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &corev1.PersistentVolumeClaim{},
			&handler.TypedEnqueueRequestForObject[*corev1.PersistentVolumeClaim]{},
			predicate.TypedFuncs[*corev1.PersistentVolumeClaim]{
				CreateFunc: func(e event.TypedCreateEvent[*corev1.PersistentVolumeClaim]) bool {
					return hasPopulationStrategy(e.Object)
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.PersistentVolumeClaim]) bool {
					if !hasPopulationStrategy(e.ObjectNew) {
						return false
					}
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase ||
						e.ObjectOld.Spec.VolumeName != e.ObjectNew.Spec.VolumeName ||
						e.ObjectOld.Annotations[service.SelectedNodeAnnotation] != e.ObjectNew.Annotations[service.SelectedNodeAnnotation]
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*corev1.PersistentVolumeClaim]) bool {
					return hasPopulationStrategy(e.Object)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("watch population target PVCs: %w", err)
	}

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &corev1.PersistentVolumeClaim{},
			handler.TypedEnqueueRequestsFromMapFunc(enqueuePopulationTargetFromHelperPVC(mgr.GetClient())),
			predicate.TypedFuncs[*corev1.PersistentVolumeClaim]{
				CreateFunc: func(e event.TypedCreateEvent[*corev1.PersistentVolumeClaim]) bool {
					return isPopulationHelperPVC(e.Object)
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.PersistentVolumeClaim]) bool {
					if !isPopulationHelperPVC(e.ObjectNew) {
						return false
					}
					return e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase ||
						e.ObjectOld.Spec.VolumeName != e.ObjectNew.Spec.VolumeName
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*corev1.PersistentVolumeClaim]) bool {
					return isPopulationHelperPVC(e.Object)
				},
			},
		),
	); err != nil {
		return fmt.Errorf("watch population helper PVCs: %w", err)
	}

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &corev1.Pod{},
			handler.TypedEnqueueRequestsFromMapFunc(enqueuePopulationTargetFromImporterPod(mgr.GetClient())),
			predicate.TypedFuncs[*corev1.Pod]{
				CreateFunc: func(e event.TypedCreateEvent[*corev1.Pod]) bool {
					return isImporterPod(e.Object)
				},
				UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Pod]) bool {
					return isImporterPod(e.ObjectNew) && e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Pod]) bool { return false },
			},
		),
	); err != nil {
		return fmt.Errorf("watch importer pods: %w", err)
	}

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &vsv1.VolumeSnapshot{},
			handler.TypedEnqueueRequestsFromMapFunc(enqueuePopulationTargetFromVolumeSnapshot(mgr.GetClient())),
			predicate.TypedFuncs[*vsv1.VolumeSnapshot]{
				CreateFunc: func(e event.TypedCreateEvent[*vsv1.VolumeSnapshot]) bool { return true },
				UpdateFunc: func(e event.TypedUpdateEvent[*vsv1.VolumeSnapshot]) bool {
					return volumeSnapshotReadyToUse(e.ObjectOld) != volumeSnapshotReadyToUse(e.ObjectNew) ||
						(e.ObjectOld.Status != nil && e.ObjectNew.Status != nil &&
							e.ObjectOld.Status.BoundVolumeSnapshotContentName != e.ObjectNew.Status.BoundVolumeSnapshotContentName)
				},
				DeleteFunc: func(e event.TypedDeleteEvent[*vsv1.VolumeSnapshot]) bool { return false },
			},
		),
	); err != nil {
		return fmt.Errorf("watch volume snapshots: %w", err)
	}

	return nil
}

func isPopulationHelperPVC(pvc *corev1.PersistentVolumeClaim) bool {
	if pvc == nil || hasPopulationStrategy(pvc) {
		return false
	}
	for _, ref := range pvc.OwnerReferences {
		if ref.Kind == "PersistentVolumeClaim" {
			return true
		}
	}
	return false
}

func isImporterPod(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	if pod.Labels[annotations.AppLabel] != annotations.CDILabelValue {
		return false
	}
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "PersistentVolumeClaim" {
			return true
		}
	}
	return false
}

func enqueuePopulationTargetFromHelperPVC(c client.Client) func(context.Context, *corev1.PersistentVolumeClaim) []reconcile.Request {
	return func(ctx context.Context, pvc *corev1.PersistentVolumeClaim) []reconcile.Request {
		return populationTargetRequests(pvc.Namespace, helperPopulationTargetName(ctx, c, pvc))
	}
}

func helperPopulationTargetName(ctx context.Context, c client.Client, helper *corev1.PersistentVolumeClaim) string {
	for _, ref := range helper.OwnerReferences {
		if ref.Kind != "PersistentVolumeClaim" {
			continue
		}
		owner, err := object.FetchObject(ctx, types.NamespacedName{Name: ref.Name, Namespace: helper.Namespace}, c, &corev1.PersistentVolumeClaim{})
		if err != nil || owner == nil || !hasPopulationStrategy(owner) {
			continue
		}
		return owner.Name
	}
	return ""
}

func enqueuePopulationTargetFromImporterPod(c client.Client) func(context.Context, *corev1.Pod) []reconcile.Request {
	return func(ctx context.Context, pod *corev1.Pod) []reconcile.Request {
		for _, ref := range pod.OwnerReferences {
			if ref.Kind != "PersistentVolumeClaim" {
				continue
			}
			target, err := object.FetchObject(ctx, types.NamespacedName{Name: ref.Name, Namespace: pod.Namespace}, c, &corev1.PersistentVolumeClaim{})
			if err != nil || target == nil || !hasPopulationStrategy(target) {
				continue
			}
			return populationTargetRequests(target.Namespace, target.Name)
		}
		return nil
	}
}

func enqueuePopulationTargetFromVolumeSnapshot(c client.Client) func(context.Context, *vsv1.VolumeSnapshot) []reconcile.Request {
	return func(ctx context.Context, vs *vsv1.VolumeSnapshot) []reconcile.Request {
		var pvcList corev1.PersistentVolumeClaimList
		if err := c.List(ctx, &pvcList, client.InNamespace(vs.Namespace)); err != nil {
			return nil
		}

		var requests []reconcile.Request
		seen := make(map[string]struct{})
		for i := range pvcList.Items {
			pvc := &pvcList.Items[i]
			if !hasPopulationStrategy(pvc) {
				continue
			}
			if pvc.Annotations[annotations.AnnPVCPopulationStrategy] != service.PopulationStrategySnapshot {
				continue
			}
			if snapshotNameFromPVC(pvc) != vs.Name {
				continue
			}
			if _, ok := seen[pvc.Name]; ok {
				continue
			}
			seen[pvc.Name] = struct{}{}
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(pvc)})
		}
		return requests
	}
}

func populationTargetRequests(namespace, name string) []reconcile.Request {
	if name == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}}}
}

func volumeSnapshotReadyToUse(vs *vsv1.VolumeSnapshot) bool {
	if vs == nil || vs.Status == nil || vs.Status.ReadyToUse == nil {
		return false
	}
	return *vs.Status.ReadyToUse
}
