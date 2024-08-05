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

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	controllerName = "vmop-controller"
)

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	logger *slog.Logger,
) (controller.Controller, error) {
	reconciler := NewReconciler()

	reconcilerCore := two_phase_reconciler.NewReconcilerCore[*ReconcilerState](
		reconciler,
		NewReconcilerState,
		two_phase_reconciler.ReconcilerOptions{
			Client:   mgr.GetClient(),
			Cache:    mgr.GetCache(),
			Recorder: mgr.GetEventRecorderFor(controllerName),
			Scheme:   mgr.GetScheme(),
			Log:      logger.With("controller", controllerName),
		})

	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:   reconcilerCore,
		RateLimiter:  workqueue.NewItemExponentialFailureRateLimiter(time.Second, 32*time.Second),
		RecoverPanic: ptr.To(true),
	})
	if err != nil {
		return nil, err
	}

	if err := reconciler.SetupController(ctx, mgr, c); err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachineOperation{}).
		WithValidator(NewValidator(logger)).
		Complete(); err != nil {
		return nil, err
	}

	logger.Info("Initialized VirtualMachineOperation controller")
	return c, nil
}
