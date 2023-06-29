package controller

import (
	"context"

	"github.com/go-logr/logr"
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
	importControllerName = "import-controller"

	// ImportTargetInUse is reason for event created when an import Pod is already owns CVMI
	ImportTargetInUse = "ImportTargetInUse"
)

func NewImportController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
	importerImage string,
	controllerNamespace string,
	dvcrSettings *cc.DVCRSettings) (controller.Controller, error) {
	reconciler := &ImporterReconciler{
		client:       mgr.GetClient(),
		recorder:     mgr.GetEventRecorderFor(importControllerName),
		scheme:       mgr.GetScheme(),
		log:          log.WithName(importControllerName),
		image:        importerImage,
		namespace:    controllerNamespace,
		dvcrSettings: dvcrSettings,
	}
	importController, err := controller.New(importControllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}
	if err := addImportControllerWatches(mgr, importController, log); err != nil {
		return nil, err
	}
	log.Info("Initialized import-controller", "image", importerImage, "namespace", controllerNamespace)
	return importController, nil
}

func addImportControllerWatches(mgr manager.Manager, c controller.Controller, log logr.Logger) error {
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
