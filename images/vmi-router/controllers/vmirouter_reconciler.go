/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

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

	virtv1alpha2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
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
				r.log.V(4).Info(fmt.Sprintf("Got CREATE event for VM %s/%s", e.Object.GetNamespace(), e.Object.GetName()))
				return true
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				r.log.V(4).Info(fmt.Sprintf("Got DELETE event for VM %s/%s", e.Object.GetNamespace(), e.Object.GetName()))
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				r.log.V(4).Info(fmt.Sprintf("Got UPDATE event for VM %s/%s", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName()))
				return true
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VM: %w", err)
	}

	return nil
}

func (r *VMRouterReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	r.log.V(4).Info(fmt.Sprintf("Got reconcile request for %s", req.String()))

	// Start with retrieving affected VMI.
	vm := &virtv1alpha2.VirtualMachine{}
	err := r.client.Get(ctx, req.NamespacedName, vm)
	if err != nil && !k8serrors.IsNotFound(err) {
		r.log.Error(err, fmt.Sprintf("fail to retrieve vm/%s", req.String()))
		return reconcile.Result{}, err
	}

	var ipAddr string
	if vm != nil {
		ipAddr = vm.Status.IPAddress
	}

	// Delete route on VM deletion.
	if vm == nil || vm.DeletionTimestamp != nil {
		r.netlinkMgr.DeleteRoute(req.NamespacedName, ipAddr)
		return reconcile.Result{}, nil
	}

	r.netlinkMgr.UpdateRoute(ctx, vm)
	return reconcile.Result{}, nil
}
