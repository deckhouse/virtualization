package vmop

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/api/v1alpha2"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

const (
	controllerName = "vmop-controller"
)

func NewVMOPController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
) (controller.Controller, error) {
	reconciler := NewVMOPReconciler()

	reconcilerCore := two_phase_reconciler.NewReconcilerCore[*VMOPReconcilerState](
		reconciler,
		NewVMOPReconcilerState,
		two_phase_reconciler.ReconcilerOptions{
			Client:   mgr.GetClient(),
			Cache:    mgr.GetCache(),
			Recorder: mgr.GetEventRecorderFor(controllerName),
			Scheme:   mgr.GetScheme(),
			Log:      log.WithName(controllerName),
		})

	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler:  reconcilerCore,
		RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(time.Second, 32*time.Second),
	})
	if err != nil {
		return nil, err
	}

	if err := reconciler.SetupController(ctx, mgr, c); err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachineIPAddressLease{}).
		WithValidator(NewVMOPValidator(log)).
		Complete(); err != nil {
		return nil, err
	}

	log.Info("Initialized VirtualMachineOperation controller")
	return c, nil
}
