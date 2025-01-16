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

package vmmaclease

import (
	"context"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmmaclease/internal"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	ControllerName = "vmmaclease-controller"
)

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	retentionDurationStr string,
) (controller.Controller, error) {
	retentionDuration, err := time.ParseDuration(retentionDurationStr)
	if err != nil {
		log.Error("Failed to parse retention duration", "err", err)
		return nil, err
	}

	handlers := []Handler{
		internal.NewProtectionHandler(),
		internal.NewRetentionHandler(retentionDuration),
		internal.NewLifecycleHandler(),
	}

	r := NewReconciler(mgr.GetClient(), handlers...)

	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       r,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
	})
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

	log.Info("Initialized VirtualMachineMACLease controller")
	return c, nil
}
