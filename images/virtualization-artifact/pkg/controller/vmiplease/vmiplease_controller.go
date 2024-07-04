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

package vmiplease

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmiplease/internal"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	controllerName = "vmiplease-controller"
)

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
	retentionDurationStr string,
) (controller.Controller, error) {
	log = log.WithName(controllerName)
	retentionDuration, err := time.ParseDuration(retentionDurationStr)
	if err != nil {
		log.Error(err, "failed to parse retention duration")
		return nil, err
	}

	handlers := []Handler{
		internal.NewProtectionHandler(log),
		internal.NewRetentionHandler(log, retentionDuration),
		internal.NewLifecycleHandler(log),
	}

	r := NewReconciler(mgr.GetClient(), log, handlers...)

	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	if err = r.SetupController(ctx, mgr, c); err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachineIPAddressLease{}).
		WithValidator(NewValidator(log)).
		Complete(); err != nil {
		return nil, err
	}

	log.Info("Initialized VirtualMachineIPLease controller")
	return c, nil
}
