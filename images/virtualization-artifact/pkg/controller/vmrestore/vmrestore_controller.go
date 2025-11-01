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

package vmrestore

import (
	"context"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmrestore/internal"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const ControllerName = "vmrestore-controller"

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
) error {
	recorder := eventrecord.NewEventRecorderLogger(mgr, ControllerName)
	reconciler := NewReconciler(
		mgr.GetClient(),
		internal.NewVirtualMachineSnapshotReadyToUseHandler(mgr.GetClient()),
		internal.NewLifeCycleHandler(mgr.GetClient(), restorer.NewSecretRestorer(mgr.GetClient()), recorder),
	)

	vmRestoreController, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       reconciler,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
		UsePriorityQueue: ptr.To(true),
	})
	if err != nil {
		return err
	}

	err = reconciler.SetupController(ctx, mgr, vmRestoreController)
	if err != nil {
		return err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachineRestore{}).
		WithValidator(NewValidator()).
		Complete(); err != nil {
		return err
	}

	log.Info("Initialized VirtualMachineRestore controller")

	return nil
}
