/*
Copyright 2025 Flant JSC

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

package watcher

import (
	"fmt"

	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type DataVolumeWatcher struct{}

func NewDataVolumeWatcher() *DataVolumeWatcher {
	return &DataVolumeWatcher{}
}

func (w *DataVolumeWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &cdiv1.DataVolume{},
			handler.TypedEnqueueRequestForOwner[*cdiv1.DataVolume](
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&v1alpha2.VirtualDisk{},
				handler.OnlyControllerOwner(),
			),
			predicate.TypedFuncs[*cdiv1.DataVolume]{
				CreateFunc: func(e event.TypedCreateEvent[*cdiv1.DataVolume]) bool { return false },
				UpdateFunc: func(e event.TypedUpdateEvent[*cdiv1.DataVolume]) bool {
					if e.ObjectOld.Status.Progress != e.ObjectNew.Status.Progress {
						return true
					}

					if e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase && e.ObjectNew.Status.Phase == cdiv1.Succeeded {
						return true
					}

					if e.ObjectOld.Status.ClaimName != e.ObjectNew.Status.ClaimName {
						return true
					}

					oldDVQuotaNotExceeded, oldOk := conditions.GetDataVolumeCondition(conditions.DVQoutaNotExceededConditionType, e.ObjectOld.Status.Conditions)
					newDVQuotaNotExceeded, newOk := conditions.GetDataVolumeCondition(conditions.DVQoutaNotExceededConditionType, e.ObjectNew.Status.Conditions)

					if !oldOk && newOk {
						return true
					}

					if oldOk && newOk && oldDVQuotaNotExceeded != newDVQuotaNotExceeded {
						return true
					}

					dvRunning := service.GetDataVolumeCondition(cdiv1.DataVolumeRunning, e.ObjectNew.Status.Conditions)
					return dvRunning != nil && (dvRunning.Reason == "Error" || dvRunning.Reason == "ImagePullFailed")
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on DV: %w", err)
	}
	return nil
}
