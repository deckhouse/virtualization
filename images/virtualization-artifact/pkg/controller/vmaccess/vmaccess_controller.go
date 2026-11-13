/*
Copyright 2026 Flant JSC

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

package vmaccess

import (
	"context"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmaccess/internal"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
)

const ControllerName = "vm-access-coordinator"

// SetupController registers the virtual machine access coordinator, which freezes and unfreezes the guest
// filesystem for the VirtualMachineAccessClaims snapshots create.
func SetupController(
	ctx context.Context,
	mgr manager.Manager,
	virtClient kubeclient.Client,
	log *log.Logger,
) error {
	r := NewReconciler(
		mgr.GetClient(),
		internal.NewProtectionHandler(),
		internal.NewLifecycleHandler(service.NewFreezeService(virtClient), eventrecord.NewEventRecorderLogger(mgr, ControllerName), time.Now),
		internal.NewCleanupHandler(),
	)

	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler: r,
		// A freeze or unfreeze request holds a worker for up to 20s, the virt-handler timeout of the call, so
		// several workers keep slow guests from delaying the unfreezes and deadlines of the others.
		MaxConcurrentReconciles: 20,
		RecoverPanic:            ptr.To(true),
		LogConstructor:          logger.NewConstructor(log),
		CacheSyncTimeout:        10 * time.Minute,
		UsePriorityQueue:        ptr.To(true),
	})
	if err != nil {
		return err
	}

	if err = r.SetupController(ctx, mgr, c); err != nil {
		return err
	}

	log.Info("Initialized virtual machine access coordinator")

	return nil
}
