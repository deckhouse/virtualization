package ipam

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

const (
	claimControllerName = "claim-controller"
)

func NewClaimController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
	vmCIDRs []string,
) (controller.Controller, error) {
	reconciler, err := NewClaimReconciler(vmCIDRs)
	if err != nil {
		return nil, err
	}

	reconcilerCore := two_phase_reconciler.NewReconcilerCore[*ClaimReconcilerState](
		reconciler,
		NewClaimReconcilerState,
		two_phase_reconciler.ReconcilerOptions{
			Client:   mgr.GetClient(),
			Cache:    mgr.GetCache(),
			Recorder: mgr.GetEventRecorderFor(claimControllerName),
			Scheme:   mgr.GetScheme(),
			Log:      log.WithName(claimControllerName),
		})

	c, err := controller.New(claimControllerName, mgr, controller.Options{
		Reconciler:  reconcilerCore,
		RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(time.Second, 32*time.Second),
	})
	if err != nil {
		return nil, err
	}

	if err = reconciler.SetupController(ctx, mgr, c); err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v2alpha1.VirtualMachineIPAddressClaim{}).
		WithValidator(NewClaimValidator(log, mgr.GetClient())).
		Complete(); err != nil {
		return nil, err
	}

	log.Info("Initialized VirtualMachineIPAddressClaim controller")
	return c, nil
}
