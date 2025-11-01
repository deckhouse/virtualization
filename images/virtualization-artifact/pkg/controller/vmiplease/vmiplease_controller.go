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
	"fmt"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmiplease/internal"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	ControllerName = "vmiplease-controller"
)

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	retentionDurationStr string,
) (controller.Controller, error) {
	recorder := eventrecord.NewEventRecorderLogger(mgr, ControllerName)
	retentionDuration, err := time.ParseDuration(retentionDurationStr)
	if err != nil {
		return nil, fmt.Errorf("parse retention duration: %w", err)
	}

	handlers := []Handler{
		internal.NewLifecycleHandler(mgr.GetClient(), recorder),
		internal.NewProtectionHandler(),
		internal.NewRetentionHandler(retentionDuration, mgr.GetClient()),
	}

	r := NewReconciler(mgr.GetClient(), handlers...)

	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       r,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
		UsePriorityQueue: ptr.To(true),
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

	log.Info("Initialized VirtualMachineIPLease controller")
	return c, nil
}
