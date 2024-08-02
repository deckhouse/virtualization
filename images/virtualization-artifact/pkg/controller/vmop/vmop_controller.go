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

package vmop

import (
	"context"
	"log/slog"
	"time"

	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmop/internal"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	controllerName = "vmop-controller"
)

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	logger *slog.Logger,
) (controller.Controller, error) {
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("controller", controllerName)

	handlers := []Handler{
		internal.NewOperationHandler(logger),
		internal.NewLifecycleHandler(logger),
		internal.NewProtectionHandler(logger),
	}

	reconciler := NewReconciler(mgr.GetClient(), logger, handlers...)

	vmopController, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:   reconciler,
		RateLimiter:  workqueue.NewItemExponentialFailureRateLimiter(time.Second, 32*time.Second),
		RecoverPanic: ptr.To(true),
	})
	if err != nil {
		return nil, err
	}

	err = reconciler.SetupController(ctx, mgr, vmopController)
	if err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&virtv2.VirtualMachineOperation{}).
		WithValidator(NewValidator(logger)).
		Complete(); err != nil {
		return nil, err
	}

	logger.Info("Initialized VirtualMachineOperation controller")
	return vmopController, nil
}
