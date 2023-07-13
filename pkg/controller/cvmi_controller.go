package controller

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
)

const (
	cvmiControllerName = "cvmi-controller"

	// ImportTargetInUse is reason for event created when an import Pod is already owns CVMI
	ImportTargetInUse = "ImportTargetInUse"

	ImporterPodVerbose    = "3"
	ImporterPodPullPolicy = string(corev1.PullAlways)
)

func NewCVMIController(
	_ context.Context,
	mgr manager.Manager,
	log logr.Logger,
	importerImage string,
	controllerNamespace string,
	dvcrSettings *cc.DVCRSettings,
) (controller.Controller, error) {
	reconciler := &CVMIReconciler{
		client:       mgr.GetClient(),
		recorder:     mgr.GetEventRecorderFor(cvmiControllerName),
		log:          log.WithName(cvmiControllerName),
		image:        importerImage,
		pullPolicy:   ImporterPodPullPolicy,
		verbose:      ImporterPodVerbose,
		namespace:    controllerNamespace,
		dvcrSettings: dvcrSettings,
	}
	cvmiController, err := controller.New(cvmiControllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}
	if err = addCVMIControllerWatches(cvmiController); err != nil {
		return nil, err
	}
	log.Info("Initialized ClusterVirtualMachineImage controller", "image", importerImage, "namespace", controllerNamespace)
	return cvmiController, nil
}

func addCVMIControllerWatches(c controller.Controller) error {
	if err := c.Watch(&source.Kind{Type: &virtv2alpha1.ClusterVirtualMachineImage{}}, &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			// ClusterVirtualMachineImage is immutable, no need to create work task for modified object.
			UpdateFunc: func(e event.UpdateEvent) bool { return false },
		},
	); err != nil {
		return err
	}

	return nil
}
