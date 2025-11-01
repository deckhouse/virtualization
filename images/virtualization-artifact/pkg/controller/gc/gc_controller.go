/*
Copyright 2024 Flant JSC

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

package gc

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

type ReconcileGCManager interface {
	New() client.Object
	ShouldBeDeleted(obj client.Object) bool
	ListForDelete(ctx context.Context, now time.Time) ([]client.Object, error)
}

func SetupGcController(
	controllerName string,
	mgr manager.Manager,
	log *log.Logger,
	schedule string,
	gcMgr ReconcileGCManager,
) error {
	log = log.With(logger.SlogController(controllerName))

	cronSource, err := NewCronSource(schedule, NewObjectLister(gcMgr.ListForDelete), log)
	if err != nil {
		return err
	}

	reconciler := NewReconciler(mgr.GetClient(),
		cronSource,
		gcMgr,
	)

	err = reconciler.SetupWithManager(controllerName, mgr, log)
	if err != nil {
		return err
	}

	log.Info("Initialized garbage collector controller")

	return nil
}
