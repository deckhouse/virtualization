package controller

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
)

const (
	cvmiControllerName = "cvmi-controller"

	ImporterPodVerbose    = "3"
	ImporterPodPullPolicy = string(corev1.PullAlways)
)

func NewCVMIController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
	importerImage string,
	controllerNamespace string,
	dvcrSettings *cc.DVCRSettings,
) (controller.Controller, error) {
	reconciler := &CVMIReconciler{
		image:        importerImage,
		verbose:      ImporterPodVerbose,
		pullPolicy:   ImporterPodPullPolicy,
		namespace:    controllerNamespace,
		dvcrSettings: dvcrSettings,
	}

	reconcilerCore := two_phase_reconciler.NewReconcilerCore[*CVMIReconcilerState](
		reconciler,
		NewCVMIReconcilerState,
		two_phase_reconciler.ReconcilerOptions{
			Client:   mgr.GetClient(),
			Cache:    mgr.GetCache(),
			Recorder: mgr.GetEventRecorderFor(cvmiControllerName),
			Scheme:   mgr.GetScheme(),
			Log:      log.WithName(cvmiControllerName),
		})

	cvmiController, err := controller.New(cvmiControllerName, mgr, controller.Options{Reconciler: reconcilerCore})
	if err != nil {
		return nil, err
	}
	if err := reconciler.SetupController(ctx, mgr, cvmiController); err != nil {
		return nil, err
	}
	log.Info("Initialized ClusterVirtualMachineImage controller", "image", importerImage, "namespace", controllerNamespace)
	return cvmiController, nil
}
