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

// Package migrationiface annotates each Node with the kernel interface name
// of a dedicated live-migration network, resolved from sdn's
// SystemNetworkNodeNetworkInterfaceAttachment + NodeNetworkInterface.
// virt-handler reads the annotation (see pkg/common/annotations.AnnMigrationIface)
// at startup to bind migration traffic to that interface.
package migrationiface

import (
	"context"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const ControllerName = "migrationiface-controller"

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	systemNetworkName string,
) (controller.Controller, error) {
	if !featuregates.Default().Enabled(featuregates.SDN) {
		log.Info("SDN feature gate is disabled, migrationiface controller is disabled")
		return nil, nil
	}
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
