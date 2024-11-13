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
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

func SetupGcController(
	controllerName string,
	mgr manager.Manager,
	log *log.Logger,
	watchSource source.Source,
	newObject NewObject,
	isNeedDelete IsNeedDelete,
) error {
	log = log.With(logger.SlogController(controllerName))
	recorder := mgr.GetEventRecorderFor(controllerName)
	reconciler := NewReconciler(mgr.GetClient(),
		recorder,
		watchSource,
		newObject,
		isNeedDelete,
	)

	err := reconciler.SetupWithManager(controllerName, mgr, log)
	if err != nil {
		return err
	}

	log.Info("Initialized garbage collector controller")

	return nil
}
