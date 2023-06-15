package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
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
	vmdControllerName = "vmd-controller"
)


type VMDReconciler struct {
	client   client.Client
	recorder record.EventRecorder
	scheme   *runtime.Scheme
	log      logr.Logger
}

// Reconcile loop for CVMIReconciler.
func (r *VMDReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	//cvmImage := &virtv1.ClusterVirtualMachineImage{}
	//if err := r.client.Get(ctx, req.NamespacedName, cvmImage); err != nil {
	//	if k8serrors.IsNotFound(err) {
	//		r.log.Info(fmt.Sprintf("Reconcile observe absent CVMI: %s, it may be deleted", req.String()))
	//		return reconcile.Result{}, nil
	//	}
	//	return reconcile.Result{}, err
	//}
	// Use cvmImage to start builder Pod.

	r.log.Info(fmt.Sprintf("Reconcile for VMD: %s", req.String()))

	return reconcile.Result{}, nil
}

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
