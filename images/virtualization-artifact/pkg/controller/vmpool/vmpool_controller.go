//go:build EE
// +build EE

/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package vmpool

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

const (
	ControllerName = "vmpool-controller"
)

// SetupController wires the VirtualMachinePool controller into the manager.
//
// The resource is gated behind the VirtualMachinePool feature gate: while the
// gate is off the controller is not set up at all (the CRD is still installed,
// so objects can be created — they simply are not reconciled). See ADR
// "VirtualMachinePool", section "Feature gate".
func SetupController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
) error {
	if !featuregates.Default().Enabled(featuregates.VirtualMachinePool) {
		return nil
	}

	client := mgr.GetClient()

	// Handlers are added by the follow-up slices (replica maintenance, template
	// propagation, reuse disks). The scaffold wires an empty chain.
	handlers := []Handler{}
	r := NewReconciler(client, handlers)

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

	log.Info("Initialized VirtualMachinePool controller")
	return nil
}
