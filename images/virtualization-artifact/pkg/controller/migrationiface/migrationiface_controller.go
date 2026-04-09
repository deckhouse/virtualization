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

// Package migrationiface implements a controller that annotates each Node
// with the kernel interface name of a dedicated live-migration network,
// resolved from a SystemNetworkNodeNetworkInterfaceAttachment provided by
// the sdn module. virt-handler reads this annotation at startup to bind
// migration traffic to the dedicated network instead of the default pod IP.
package migrationiface

import (
	"context"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const (
	ControllerName = "migrationiface-controller"

	// MigrationIfaceAnnotation holds the kernel interface name on a Node that
	// virt-handler should bind live-migration traffic to. Empty/absent means
	// no override (upstream behavior).
	MigrationIfaceAnnotation = "network.virtualization.deckhouse.io/migration-iface"
)

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	systemNetworkName string,
) (controller.Controller, error) {
	if systemNetworkName == "" {
		log.Info("MIGRATION_SYSTEM_NETWORK_NAME is empty, migrationiface controller is disabled")
		return nil, nil
	}

	r := NewReconciler(mgr.GetClient(), systemNetworkName, log)

	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       r,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
	})
	if err != nil {
		return nil, err
	}

	if err = r.SetupController(ctx, mgr, c); err != nil {
		return nil, err
	}

	log.Info("Initialized migrationiface controller", "systemNetwork", systemNetworkName)
	return c, nil
}
