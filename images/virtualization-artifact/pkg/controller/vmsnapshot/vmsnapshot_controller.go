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

package vmsnapshot

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
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmsnapshot/internal"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	vmsnapshotcollector "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/vmsnapshot"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const ControllerName = "vmsnapshot-controller"

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	virtClient kubeclient.Client,
) error {
	protection := service.NewProtectionService(mgr.GetClient(), v1alpha2.FinalizerVMSnapshotProtection)
	recorder := eventrecord.NewEventRecorderLogger(mgr, ControllerName)
	snapshotter := service.NewSnapshotService(virtClient, mgr.GetClient(), protection)

	reconciler := NewReconciler(
		mgr.GetClient(),
		internal.NewVirtualMachineReadyHandler(snapshotter),
		internal.NewLifeCycleHandler(recorder, snapshotter, restorer.NewSecretRestorer(mgr.GetClient()), mgr.GetClient()),
	)

	vmSnapshotController, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       reconciler,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
		UsePriorityQueue: ptr.To(true),
	})
	if err != nil {
		return err
	}

	err = reconciler.SetupController(ctx, mgr, vmSnapshotController)
	if err != nil {
		return err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachineSnapshot{}).
		WithValidator(NewValidator()).
		Complete(); err != nil {
		return err
	}

	vmsnapshotcollector.SetupCollector(mgr.GetCache(), metrics.Registry, log)

	log.Info("Initialized VirtualMachineSnapshot controller")

	return nil
}
