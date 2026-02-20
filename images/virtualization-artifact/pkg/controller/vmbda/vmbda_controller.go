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
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmbda/internal"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vmbda/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	vmbdametrics "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/vmbda"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const ControllerName = "vmbda-controller"

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	virtClient kubeclient.Client,
	lg *log.Logger,
	ns string,
) (controller.Controller, error) {
	attacher := intsvc.NewAttachmentService(mgr.GetClient(), virtClient, ns)
	blockDeviceService := service.NewBlockDeviceService(mgr.GetClient())

	reconciler := NewReconciler(
		mgr.GetClient(),
		internal.NewBlockDeviceLimiter(blockDeviceService),
		internal.NewBlockDeviceReadyHandler(attacher),
		internal.NewVirtualMachineReadyHandler(attacher),
		internal.NewLifeCycleHandler(attacher),
		internal.NewDeletionHandler(attacher, mgr.GetClient()),
	)

	vmbdaController, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       reconciler,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(lg),
		CacheSyncTimeout: 10 * time.Minute,
		UsePriorityQueue: ptr.To(true),
	})
	if err != nil {
		return nil, err
	}

	err = reconciler.SetupController(ctx, mgr, vmbdaController)
	if err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachineBlockDeviceAttachment{}).
		WithValidator(NewValidator(attacher, blockDeviceService, lg)).
		Complete(); err != nil {
		return nil, err
	}

	vmbdametrics.SetupCollector(mgr.GetCache(), metrics.Registry, lg)

	log.Info("Initialized VirtualMachineBlockDeviceAttachment controller")

	return vmbdaController, nil
}
