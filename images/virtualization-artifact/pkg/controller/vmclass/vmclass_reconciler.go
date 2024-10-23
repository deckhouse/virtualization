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
	"errors"
	"fmt"
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

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Handler interface {
	Handle(ctx context.Context, s state.VirtualMachineClassState) (reconcile.Result, error)
	Name() string
}

func NewReconciler(client client.Client, handlers ...Handler) *Reconciler {
	return &Reconciler{
		client:   client,
		handlers: handlers,
	}
}

type Reconciler struct {
	client   client.Client
	handlers []Handler
}

func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(source.Kind(mgr.GetCache(),
		&virtv2.VirtualMachineClass{}),
		&handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("error setting watch on VMClass: %w", err)
	}
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &corev1.Node{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
			node, ok := object.(*corev1.Node)
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
						NamespacedName: common.NamespacedName(&class),
					})
					continue
				}
				if !common.MatchLabels(node.GetLabels(), class.Spec.NodeSelector.MatchLabels) {
					continue
				}
				ns, err := nodeaffinity.NewNodeSelector(&corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: class.Spec.NodeSelector.MatchExpressions}},
				})
				if err != nil || !ns.Match(node) {
					continue
				}
				result = append(result, reconcile.Request{
					NamespacedName: common.NamespacedName(&class),
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
	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	class := service.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := class.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if class.IsEmpty() {
		log.Info("Reconcile observe an absent VirtualMachineClass: it may be deleted")
		return reconcile.Result{}, nil
	}
	s := state.New(r.client, class)

	log.Debug("Start reconcile VMClass")

	var result reconcile.Result
	var handlerErr error

	for _, h := range r.handlers {
		log.Debug("Run handler", logger.SlogHandler(h.Name()))

		var res reconcile.Result
		res, err = h.Handle(ctx, s)
		if err != nil {
			log.Error("The handler failed with an error", logger.SlogErr(err), logger.SlogHandler(h.Name()))
			handlerErr = errors.Join(handlerErr, err)
		}
		result = service.MergeResults(result, res)
	}
	if handlerErr != nil {
		err = class.Update(ctx)
		if err != nil {
			log.Error("Failed to update VirtualMachineClass")
		}
		return reconcile.Result{}, handlerErr
	}
	err = class.Update(ctx)
	if err != nil {
		log.Error("Failed to update VirtualMachineClass")
		return reconcile.Result{}, err
	}

	log.Debug("Finished reconcile VirtualMachineClass")
	return result, nil
}

func (r *Reconciler) factory() *virtv2.VirtualMachineClass {
	return &virtv2.VirtualMachineClass{}
}

func (r *Reconciler) statusGetter(obj *virtv2.VirtualMachineClass) virtv2.VirtualMachineClassStatus {
	return obj.Status
}
