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

	virtv1 "github.com/deckhouse/virtualization-controller/apis/v1alpha1"
)

const (
	vmdControllerName = "vmd-controller"
)

func NewVMDController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger) (controller.Controller, error) {
	reconciler := &VMDReconciler{
		client:   mgr.GetClient(),
		recorder: mgr.GetEventRecorderFor(vmdControllerName),
		scheme:   mgr.GetScheme(),
		log:      log.WithName(vmdControllerName),
	}
	c, err := controller.New(vmdControllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}
	if err := addVMDControllerWatches(mgr, c, log); err != nil {
		return nil, err
	}
	log.Info("Initialized VirtualMachineDisk controller")
	return c, nil
}

func addVMDControllerWatches(mgr manager.Manager, c controller.Controller, log logr.Logger) error {
	if err := c.Watch(&source.Kind{Type: &virtv1.VirtualMachineDisk{}}, &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	); err != nil {
		return err
	}

	return nil
}
