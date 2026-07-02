//go:build EE
// +build EE

/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package main

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

// setupEnterpriseControllers wires controllers that ship only in paid editions
// (EE/SE+). It is compiled into EE builds; the CE build uses the no-op stub in
// setup_enterprise_ce.go.
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
