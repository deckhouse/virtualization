package controller

import (
	"context"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	vmControllerName = "vm-controller"
)

func NewVMController(ctx context.Context, mgr manager.Manager, log logr.Logger) (controller.Controller, error) {
	reconciler := &VMReconciler{}
	reconcilerCore := two_phase_reconciler.NewReconcilerCore[*VMReconcilerState](
		reconciler,
		NewVMReconcilerState,
		two_phase_reconciler.ReconcilerOptions{
			Client:   mgr.GetClient(),
			Cache:    mgr.GetCache(),
			Recorder: mgr.GetEventRecorderFor(vmControllerName),
			Scheme:   mgr.GetScheme(),
			Log:      log.WithName(vmControllerName),
		})

	c, err := controller.New(vmControllerName, mgr, controller.Options{Reconciler: reconcilerCore})
	if err != nil {
		return nil, err
	}
	if err := reconciler.SetupController(ctx, mgr, c); err != nil {
		return nil, err
	}
	log.Info("Initialized VirtualMachine controller")
	return c, nil
}
