package controllers

import (
	"context"
	"fmt"

	virtv1alpha2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"vmi-router/netlinkmanager"
)

type VMRouterReconciler struct {
	client     client.Client
	cache      cache.Cache
	recorder   record.EventRecorder
	scheme     *runtime.Scheme
	log        logr.Logger
	netlinkMgr *netlinkmanager.Manager
}

func (r *VMRouterReconciler) SetupWatches(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &virtv1alpha2.VirtualMachine{}), &handler.EnqueueRequestForObject{},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				r.log.Info("Got CREATE event for VM %s/%s", e.Object.GetNamespace(), e.Object.GetName())
				return true
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				r.log.Info("Got DELETE event for VM %s/%s", e.Object.GetNamespace(), e.Object.GetName())
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Handle VM update if IPAddress is changed.
				oldVM := e.ObjectOld.(*virtv1alpha2.VirtualMachine)
				newVM := e.ObjectNew.(*virtv1alpha2.VirtualMachine)
				return oldVM.Status.IPAddress != newVM.Status.IPAddress
				// log.Info("Got UPDATE event for VM %s/%s", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName())
				// return true
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VM: %w", err)
	}

	return nil
}

func (r *VMRouterReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	r.log.Info(fmt.Sprintf("Got request for %s", req.String()))

	// Start with retrieving affected VMI.
	var vm virtv1alpha2.VirtualMachine
	var isAbsent bool
	err := r.client.Get(ctx, req.NamespacedName, &vm)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			isAbsent = true
		} else {
			r.log.Error(err, fmt.Sprintf("fail to retrieve vm/%s", req.String()))
			return reconcile.Result{}, err
		}
	}

	// Delete route on VM deletion.
	if vm.GetDeletionTimestamp() != nil {
		r.netlinkMgr.DeleteRoute(&vm)
		return reconcile.Result{}, nil
	}

	if isAbsent {
		r.netlinkMgr.DeleteRoute(nil)
		return reconcile.Result{}, nil
	}

	r.netlinkMgr.UpdateRoute(ctx, &vm)
	return reconcile.Result{}, nil
}
