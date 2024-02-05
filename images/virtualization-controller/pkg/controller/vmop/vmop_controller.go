package vmop

import (
	"context"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/go-logr/logr"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"time"
)

const (
	vmopControllerName = "vmop-controller"
	vmopShortName      = "vmop"
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
			Recorder: mgr.GetEventRecorderFor(vmopControllerName),
			Scheme:   mgr.GetScheme(),
			Log:      log.WithName(vmopControllerName),
		})

	c, err := controller.New(vmopControllerName, mgr, controller.Options{
		Reconciler:  reconcilerCore,
		RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(time.Second, 32*time.Second),
	})
	if err != nil {
		return nil, err
	}
	if err := reconciler.SetupController(ctx, mgr, c); err != nil {
		return nil, err
	}
	log.Info("Initialized VirtualMachineOperation controller")
	return c, nil
}
