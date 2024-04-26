/*
Copyright 2023 Flant JSC

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

package controllers

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"vmi-router/netlinkmanager"
)

const controllerName = "vm-cilium-router"

func NewVMRouterController(
	mgr manager.Manager,
	log logr.Logger,
	netlinkMgr *netlinkmanager.Manager,
) error {
	reconciler := &VMRouterReconciler{
		client:     mgr.GetClient(),
		cache:      mgr.GetCache(),
		recorder:   mgr.GetEventRecorderFor(controllerName),
		scheme:     mgr.GetScheme(),
		log:        log.WithName(controllerName),
		netlinkMgr: netlinkMgr,
	}

	// Add new controller to manager.
	ctrl, err := controller.New(controllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return err
	}

	// Add watches to controller.
	if err = reconciler.SetupWatches(mgr, ctrl); err != nil {
		return err
	}

	log.Info("Initialized VMI Router controller")
	return nil
}
