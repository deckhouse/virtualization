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

package vdsnapshot

import (
	"context"
	"log/slog"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vdsnapshot/internal"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const ControllerName = "vdsnapshot-controller"

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *slog.Logger,
	virtClient kubeclient.Client,
) (controller.Controller, error) {
	log = log.With(logger.SlogController(ControllerName))

	protection := service.NewProtectionService(mgr.GetClient(), virtv2.FinalizerVDSnapshotProtection)
	freezer := service.NewSnapshotService(virtClient, mgr.GetClient(), protection)

	reconciler := NewReconciler(
		mgr.GetClient(),
		internal.NewVirtualDiskReadyHandler(freezer),
		internal.NewLifeCycleHandler(freezer),
		internal.NewDeletionHandler(freezer),
	)

	vdSnapshotController, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       reconciler,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
	})
	if err != nil {
		return nil, err
	}

	err = reconciler.SetupController(ctx, mgr, vdSnapshotController)
	if err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&virtv2.VirtualDiskSnapshot{}).
		WithValidator(NewValidator(log)).
		Complete(); err != nil {
		return nil, err
	}

	log.Info("Initialized VirtualDiskSnapshot controller")

	return vdSnapshotController, nil
}
