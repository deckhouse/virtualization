package controller

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/controller/ipam"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	vmControllerName = "vm-controller"
)

func NewVMController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
	dvcrSettings *dvcr.Settings,
) (controller.Controller, error) {
	reconciler := &VMReconciler{
		dvcrSettings: dvcrSettings,
		ipam:         ipam.New(),
	}
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

	if err = reconciler.SetupController(ctx, mgr, c); err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachine{}).
		WithValidator(NewVMValidator(ipam.New(), mgr.GetClient(), log)).
		Complete(); err != nil {
		return nil, err
	}

	log.Info("Initialized VirtualMachine controller")
	return c, nil
}
