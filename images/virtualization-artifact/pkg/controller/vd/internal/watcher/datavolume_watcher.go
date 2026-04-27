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
	"strings"

	"github.com/deckhouse/deckhouse/pkg/log"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type DataVolumeWatcher struct {
	logger *log.Logger
}

func NewDataVolumeWatcher() *DataVolumeWatcher {
	return &DataVolumeWatcher{
		logger: log.Default().With("watcher", strings.ToLower("DataVolume")),
	}
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
					w.logger.Info("[GOGOGO] [DataVolumeWatcher] Event observed")

					if e.ObjectOld.Status.Progress != e.ObjectNew.Status.Progress {
						w.logger.Info("[GOGOGO] [DataVolumeWatcher] Event observed [e.ObjectOld.Status.Progress != e.ObjectNew.Status.Progress]")
						return true
					}

					if e.ObjectOld.Status.Phase != e.ObjectNew.Status.Phase && e.ObjectNew.Status.Phase == cdiv1.Succeeded {
						w.logger.Info("[GOGOGO] [DataVolumeWatcher] Event observed [e.ObjectNew.Status.Phase == cdiv1.Succeeded]")
						return true
					}

					if e.ObjectOld.Status.ClaimName != e.ObjectNew.Status.ClaimName {
						w.logger.Info("[GOGOGO] [DataVolumeWatcher] Event observed [e.ObjectOld.Status.ClaimName != e.ObjectNew.Status.ClaimName]")
						return true
					}

					oldDVQuotaNotExceeded, oldOk := conditions.GetDataVolumeCondition(conditions.DVQoutaNotExceededConditionType, e.ObjectOld.Status.Conditions)
					newDVQuotaNotExceeded, newOk := conditions.GetDataVolumeCondition(conditions.DVQoutaNotExceededConditionType, e.ObjectNew.Status.Conditions)

					if !oldOk && newOk {
						w.logger.Info("[GOGOGO] [DataVolumeWatcher] Event observed [quota]")
						return true
					}

					if oldOk && newOk && oldDVQuotaNotExceeded != newDVQuotaNotExceeded {
						w.logger.Info("[GOGOGO] [DataVolumeWatcher] Event observed [quota 2]")
						return true
					}

					oldDVRunning, _ := conditions.GetDataVolumeCondition(conditions.DVRunningConditionType, e.ObjectOld.Status.Conditions)
					newDVRunning, _ := conditions.GetDataVolumeCondition(conditions.DVRunningConditionType, e.ObjectNew.Status.Conditions)

					if oldDVRunning.Reason != newDVRunning.Reason {
						w.logger.Info("[GOGOGO] [DataVolumeWatcher] Event observed [oldDVRunning.Reason != newDVRunning.Reason]")
						return true
					}

					f := newDVRunning.Reason == "Error" || newDVRunning.Reason == "ImagePullFailed"
					if f {
						w.logger.Info("[GOGOGO] [DataVolumeWatcher] Event observed [oldDVRunning.Reason != newDVRunning.Reason]")
						return true
					}

					w.logger.Info("[GOGOGO] [DataVolumeWatcher] Event observed [FALSE]")

					return false
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on DV: %w", err)
	}
	return nil
}
