package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

const (
	vmiControllerName = "vmi-controller"
)

func NewVMIController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
	importerImage string,
	uploaderImage string,
	dvcrSettings *cc.DVCRSettings,
) (controller.Controller, error) {
	reconciler := &VMIReconciler{
		importerImage: importerImage,
		uploaderImage: uploaderImage,
		verbose:       ImporterPodVerbose,
		pullPolicy:    ImporterPodPullPolicy,
		dvcrSettings:  dvcrSettings,
	}
	reconcilerCore := two_phase_reconciler.NewReconcilerCore[*VMIReconcilerState](
		reconciler,
		NewVMIReconcilerState,
		two_phase_reconciler.ReconcilerOptions{
			Client:   mgr.GetClient(),
			Cache:    mgr.GetCache(),
			Recorder: mgr.GetEventRecorderFor(vmiControllerName),
			Scheme:   mgr.GetScheme(),
			Log:      log.WithName(vmiControllerName),
		})

	c, err := controller.New(vmiControllerName, mgr, controller.Options{
		Reconciler:  reconcilerCore,
		RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(time.Second, 32*time.Second),
	})
	if err != nil {
		return nil, err
	}
	if err := reconciler.SetupController(ctx, mgr, c); err != nil {
		return nil, err
	}
	log.Info("Initialized VirtualMachineImage controller")
	return c, nil
}
