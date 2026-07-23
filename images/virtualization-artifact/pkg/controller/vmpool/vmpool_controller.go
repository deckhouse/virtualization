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

package vmpool

import (
	"context"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/expectations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/handler"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const (
	ControllerName = "vmpool-controller"
)

// SetupController wires the VirtualMachinePool controller into the manager.
//
// The resource is available only in paid editions (EE/SE+): in CE the controller
// is not set up at all (the CRD is still installed, so objects can be created —
// they simply are not reconciled). The edition check rides on the
// VirtualMachinePool feature gate, which is locked on in EE/SE+ and off in CE.
// See ADR "VirtualMachinePool", section "Feature gate".
func SetupController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	virtualMachineCIDRs []string,
) error {
	if !featuregates.Default().Enabled(featuregates.VirtualMachinePool) {
		return nil
	}

	client := mgr.GetClient()

	// exp guards against a lagging informer cache causing double create/delete of
	// anonymous replicas. It is shared between the reconcile handlers and the
	// member watcher that observes creations/deletions.
	exp := expectations.New()
	recorder := eventrecord.NewEventRecorderLogger(mgr, ControllerName)

	handlers := []Handler{
		handler.NewTemplateHandler(client),
		handler.NewSyncHandler(client, exp, recorder),
		handler.NewDisksHandler(client),
	}
	r := NewReconciler(client, exp, handlers)

	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       r,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
		UsePriorityQueue: ptr.To(true),
	})
	if err != nil {
		return err
	}

	if err = r.SetupController(ctx, mgr, c); err != nil {
		return err
	}

	// Guards anonymous scale-down for scaleDownPolicy: Explicit.
	SetupScaleWebhook(mgr)
	// Validates the embedded virtualMachineTemplate and virtualDiskTemplates specs
	// on pool create/update.
	if err = SetupValidationWebhook(mgr, log, virtualMachineCIDRs); err != nil {
		return err
	}

	log.Info("Initialized VirtualMachinePool controller")
	return nil
}
