package controller

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/api/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

const VMBDAControllerName = "vmbda-controller"

func NewVMBDAController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
	controllerNamespace string,
) (controller.Controller, error) {
	reconciler := NewVMBDAReconciler(controllerNamespace)

	reconcilerCore := two_phase_reconciler.NewReconcilerCore[*VMBDAReconcilerState](
		reconciler,
		NewVMBDAReconcilerState,
		two_phase_reconciler.ReconcilerOptions{
			Client:   mgr.GetClient(),
			Cache:    mgr.GetCache(),
			Recorder: mgr.GetEventRecorderFor(VMBDAControllerName),
			Scheme:   mgr.GetScheme(),
			Log:      log.WithName(VMBDAControllerName),
		})

	cvmiController, err := controller.New(VMBDAControllerName, mgr, controller.Options{Reconciler: reconcilerCore})
	if err != nil {
		return nil, err
	}
	if err = reconciler.SetupController(ctx, mgr, cvmiController); err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachineBlockDeviceAttachment{}).
		WithValidator(NewVMBDAValidator(log)).
		Complete(); err != nil {
		return nil, err
	}

	log.Info("Initialized VirtualMachineBlockDeviceAttachment controller")

	return cvmiController, nil
}
