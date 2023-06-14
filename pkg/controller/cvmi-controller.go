package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	virtv1 "github.com/deckhouse/virtualization-controller/apis/v1alpha1"
)

const (
	cvmiControllerName = "cvmi-controller"
)

type CVMIReconciler struct {
	client   client.Client
	recorder record.EventRecorder
	scheme   *runtime.Scheme
	log      logr.Logger
}

// Reconcile loop for CVMIReconciler.
func (r *CVMIReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	cvmImage := &virtv1.ClusterVirtualMachineImage{}
	if err := r.client.Get(ctx, req.NamespacedName, cvmImage); err != nil {
		if k8serrors.IsNotFound(err) {
			r.log.Info(fmt.Sprintf("Reconcile observe absent CVMI: %s, it may be deleted", req.String()))
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	// Use cvmImage to start builder Pod.
	r.log.Info(fmt.Sprintf("Reconcile for CVMI: %s", req.String()))

	return reconcile.Result{}, nil
}

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
	if err := c.Watch(&source.Kind{Type: &virtv1.ClusterVirtualMachineImage{}}, &handler.EnqueueRequestForObject{},
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
