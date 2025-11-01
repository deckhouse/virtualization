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

package internal

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmcontroller "github.com/deckhouse/virtualization-controller/pkg/controller/vm"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type SemaphoreHandler struct {
	mgr           manager.Manager
	dvcrSettings  *dvcr.Settings
	firmwareImage string
	logFactory    logger.Factory

	controllers map[string]chan reconcile.Request
}

func NewSemaphoreHandler(
	mgr manager.Manager,
	dvcrSettings *dvcr.Settings,
	firmwareImage string,
	logFactory logger.Factory,
) *SemaphoreHandler {
	return &SemaphoreHandler{
		mgr:           mgr,
		dvcrSettings:  dvcrSettings,
		firmwareImage: firmwareImage,
		logFactory:    logFactory,
		controllers:   make(map[string]chan reconcile.Request),
	}
}

func (h *SemaphoreHandler) Handle(ctx context.Context, vm *v1alpha2.VirtualMachine) (reconcile.Result, error) {
	logger.FromContext(ctx).Warn("[test][SUPER] STARTED")

	queue, ok := h.controllers[vm.Namespace]
	if !ok {
		logger.FromContext(ctx).Warn("[test][SUPER] START NEW NAMESPACED CONTROLLER")

		queue = make(chan reconcile.Request)

		err := vmcontroller.SetupController(ctx, h.mgr, h.logFactory(vmcontroller.ControllerName+"-"+vm.Namespace), h.dvcrSettings, h.firmwareImage, vm.Namespace, queue)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("setup vm controller for %q: %w", vm.Namespace, err)
		}

		h.controllers[vm.Namespace] = queue
	}

	logger.FromContext(ctx).Warn("[test][SUPER] PUSH REQUEST TO QUEUE")

	queue <- reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      vm.Name,
		Namespace: vm.Namespace,
	}}

	logger.FromContext(ctx).Warn("[test][SUPER] FINISHED")

	return reconcile.Result{}, nil
}
