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

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

const (
	cvmiControllerName = "cvmi-controller"
)

func NewCVMIController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger) (controller.Controller, error) {
	reconciler := &CVMIReconciler{
		client:   mgr.GetClient(),
		recorder: mgr.GetEventRecorderFor(cvmiControllerName),
		scheme:   mgr.GetScheme(),
		log:      log.WithName(cvmiControllerName),
	}
	CVMIController, err := controller.New(cvmiControllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}
	if err := addCVMIControllerWatches(mgr, CVMIController, log); err != nil {
		return nil, err
	}
	log.Info("Initialized ClusterVirtualMachineImage controller")
	return CVMIController, nil
}

func addCVMIControllerWatches(mgr manager.Manager, c controller.Controller, log logr.Logger) error {
	if err := c.Watch(&source.Kind{Type: &virtv2.ClusterVirtualMachineImage{}}, &handler.EnqueueRequestForObject{},
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
