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

package mainippool

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/mainippool/internal/handler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/mainippool/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const ControllerName = "mainippool-controller"

// SetupController maintains the d8-virtualization ClusterIPAddressPool from
// virtualMachineCIDRs and binds it to the PrimaryCNI ClusterNetwork "main".
// Without configured CIDRs there is nothing to manage: the pool has no addresses
// to give.
func SetupController(ctx context.Context, mgr manager.Manager, lg *log.Logger, virtualMachineCIDRs []string) error {
	if len(virtualMachineCIDRs) == 0 {
		return nil
	}

	// The controller watches a resource the sdn module brings: without the module
	// the kind is not registered and the manager fails to start.
	if !featuregates.Default().Enabled(featuregates.SDN) {
		lg.Info("SDN feature gate is disabled, mainippool controller is disabled")
		return nil
	}

	client := mgr.GetClient()

	r := NewReconciler(
		[]Handler{
			handler.NewPoolHandler(client, virtualMachineCIDRs),
			handler.NewPoolRefHandler(client),
		},
		[]Watcher{
			watcher.NewClusterNetworkWatcher(),
		},
	)

	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       r,
		RecoverPanic:     func() *bool { b := true; return &b }(),
		LogConstructor:   logger.NewConstructor(lg),
		CacheSyncTimeout: 10 * time.Minute,
	})
	if err != nil {
		return err
	}

	if err := r.SetupController(ctx, mgr, c); err != nil {
		return err
	}

	lg.Info("Initialized mainippool controller")
	return nil
}
