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

package vmbda

import (
	"context"
	"log/slog"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmbda/internal"
	vmbdametrics "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/vmbda"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const ControllerName = "vmbda-controller"

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
	ns string,
) (controller.Controller, error) {
	logger := slog.Default().With("controller", ControllerName)

	attacher := service.NewAttachmentService(mgr.GetClient(), ns)

	reconciler := NewReconciler(
		mgr.GetClient(),
		logger,
		internal.NewBlockDeviceReadyHandler(attacher),
		internal.NewVirtualMachineReadyHandler(attacher),
		internal.NewLifeCycleHandler(logger, attacher),
		internal.NewDeletionHandler(),
	)

	vmbdaController, err := controller.New(ControllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}

	err = reconciler.SetupController(ctx, mgr, vmbdaController)
	if err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&virtv2.VirtualMachineBlockDeviceAttachment{}).
		WithValidator(NewValidator(attacher)).
		Complete(); err != nil {
		return nil, err
	}

	vmbdametrics.SetupCollector(&lister{cache: mgr.GetCache()}, metrics.Registry)

	log.Info("Initialized VirtualMachineBlockDeviceAttachment controller")

	return vmbdaController, nil
}

type lister struct {
	cache cache.Cache
}

func (l lister) List() ([]virtv2.VirtualMachineBlockDeviceAttachment, error) {
	vmbdas := virtv2.VirtualMachineBlockDeviceAttachmentList{}
	err := l.cache.List(context.Background(), &vmbdas)
	if err != nil {
		return nil, err
	}
	return vmbdas.Items, nil
}
