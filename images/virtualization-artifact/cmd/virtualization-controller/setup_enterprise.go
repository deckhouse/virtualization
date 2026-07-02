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

package main

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

// setupEnterpriseControllers wires controllers for paid-edition (EE/SE+)
// features. Like the other edition-gated controllers (e.g. VolumeMigration), the
// code always compiles; each controller self-gates on its feature gate at setup
// (the VirtualMachinePool gate is locked off in CE), so this is a no-op there.
func setupEnterpriseControllers(
	ctx context.Context,
	mgr manager.Manager,
	logLevel, logOutput string,
	logDebugVerbosity int,
	logDebugControllerList []string,
) error {
	vmpoolLogger := logger.NewControllerLogger(vmpool.ControllerName, logLevel, logOutput, logDebugVerbosity, logDebugControllerList)
	if err := vmpool.SetupController(ctx, mgr, vmpoolLogger); err != nil {
		return err
	}
	// Guards anonymous scale-down for scaleDownPolicy: Explicit. Self-gated by
	// the VirtualMachinePool feature gate.
	vmpool.SetupScaleWebhook(mgr)
	return nil
}
