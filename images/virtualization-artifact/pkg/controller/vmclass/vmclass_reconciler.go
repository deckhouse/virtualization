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

package vmclass

import (
	"context"
	"fmt"
	"reflect"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, s state.VirtualMachineClassState) (reconcile.Result, error)
	Name() string
}

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

func NewReconciler(controllerNamespace string, client client.Client, handlers ...Handler) *Reconciler {
	return &Reconciler{
		controllerNamespace: controllerNamespace,
		client:              client,
		handlers:            handlers,
	}
}

type Reconciler struct {
	controllerNamespace string
	client              client.Client
	handlers            []Handler
}

func (r *Reconciler) SetupController(ctx context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(source.Kind(mgr.GetCache(),
		&virtv2.VirtualMachineClass{}),
		&handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("error setting watch on VMClass: %w", err)
	}
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &corev1.Node{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			node, ok := obj.(*corev1.Node)
			if !ok {
				return nil
			}
			var result []reconcile.Request

			classList := &virtv2.VirtualMachineClassList{}
			err := mgr.GetClient().List(ctx, classList)
			if err != nil {
				return nil
			}

			for _, class := range classList.Items {
				if slices.Contains(class.Status.AvailableNodes, node.GetName()) {
					result = append(result, reconcile.Request{
						NamespacedName: object.NamespacedName(&class),
					})
					continue
				}
				if !annotations.MatchLabels(node.GetLabels(), class.Spec.NodeSelector.MatchLabels) {
					continue
				}
				ns, err := nodeaffinity.NewNodeSelector(&corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: class.Spec.NodeSelector.MatchExpressions}},
				})
				if err != nil || !ns.Match(node) {
					continue
				}
				result = append(result, reconcile.Request{
					NamespacedName: object.NamespacedName(&class),
				})
			}
			return result
		}),
		predicate.Or(
			predicate.LabelChangedPredicate{},
			predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool { return true },
				DeleteFunc: func(e event.DeleteEvent) bool { return true },
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldNode := e.ObjectOld.(*corev1.Node)
					newNode := e.ObjectNew.(*corev1.Node)
					if !oldNode.Status.Allocatable[corev1.ResourceCPU].Equal(newNode.Status.Allocatable[corev1.ResourceCPU]) {
						return true
					}
					if !oldNode.Status.Allocatable[corev1.ResourceMemory].Equal(newNode.Status.Allocatable[corev1.ResourceMemory]) {
						return true
					}
					return false
				},
			},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on Node: %w", err)
	}

	for _, w := range []Watcher{
		watcher.NewVirtualMachinesWatcher(),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	vmClass := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vmClass.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vmClass.IsEmpty() {
		log.Info("Reconcile observe an absent VirtualMachineClass: it may be deleted")
		return reconcile.Result{}, nil
	}

	s := state.New(r.client, r.controllerNamespace, vmClass)

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, s)
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		vmClass.Changed().Status.ObservedGeneration = vmClass.Changed().Generation

		return vmClass.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) factory() *virtv2.VirtualMachineClass {
	return &virtv2.VirtualMachineClass{}
}

func (r *Reconciler) statusGetter(obj *virtv2.VirtualMachineClass) virtv2.VirtualMachineClassStatus {
	return obj.Status
}
