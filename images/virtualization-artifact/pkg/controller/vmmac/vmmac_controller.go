/*
Copyright 2025 Flant JSC

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

package vmmac

import (
	"context"
	"fmt"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmmac/internal"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmmac/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	ControllerName = "vmmac-controller"
)

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	clusterUUID string,
	virtClient kubeclient.Client,
) (controller.Controller, error) {
	recorder := eventrecord.NewEventRecorderLogger(mgr, ControllerName)
	macService := service.NewMACAddressService(clusterUUID, mgr.GetClient(), virtClient)
	if macService == nil {
		return nil, fmt.Errorf("failed to create MAC address service")
	}

	handlers := []Handler{
		internal.NewBoundHandler(macService, mgr.GetClient(), recorder),
		internal.NewAttachedHandler(recorder, mgr.GetClient()),
		internal.NewLifecycleHandler(recorder),
		internal.NewProtectionHandler(),
		internal.NewDeletionHandler(mgr.GetClient()),
	}

	r, err := NewReconciler(mgr.GetClient(), handlers...)
	if err != nil {
		return nil, err
	}

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
		For(&v1alpha2.VirtualMachineMACAddress{}).
		WithValidator(NewValidator(log, mgr.GetClient(), macService)).
		Complete(); err != nil {
		return nil, err
	}

	log.Info("Initialized VirtualMachineMAC controller")
	return c, nil
}
