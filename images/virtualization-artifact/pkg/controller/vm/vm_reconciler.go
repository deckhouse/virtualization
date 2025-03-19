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

package vm

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/watcher"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type Handler interface {
	Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error)
	Name() string
}

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
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
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}), &handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("error setting watch on VM: %w", err)
	}

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachine{}),
		handler.EnqueueRequestForOwner(
			mgr.GetScheme(),
			mgr.GetRESTMapper(),
			&virtv2.VirtualMachine{},
			handler.OnlyControllerOwner(),
		),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVM := e.ObjectOld.(*virtv1.VirtualMachine)
				newVM := e.ObjectNew.(*virtv1.VirtualMachine)
				return oldVM.Status.PrintableStatus != newVM.Status.PrintableStatus ||
					oldVM.Status.Ready != newVM.Status.Ready ||
					oldVM.Annotations[annotations.AnnVmStartRequested] != newVM.Annotations[annotations.AnnVmStartRequested] ||
					oldVM.Annotations[annotations.AnnVmRestartRequested] != newVM.Annotations[annotations.AnnVmRestartRequested]
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}

	// Subscribe on Kubevirt VirtualMachineInstances to update our VM status.
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachineInstance{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, vmi client.Object) []reconcile.Request {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      vmi.GetName(),
						Namespace: vmi.GetNamespace(),
					},
				},
			}
		}),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVM := e.ObjectOld.(*virtv1.VirtualMachineInstance)
				newVM := e.ObjectNew.(*virtv1.VirtualMachineInstance)
				return !reflect.DeepEqual(oldVM.Status, newVM.Status)
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachine: %w", err)
	}

	// Watch for Pods created on behalf of VMs. Handle only changes in status.phase.
	// Pod tracking is required to detect when Pod becomes Completed after guest initiated reset or shutdown.
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &corev1.Pod{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, pod client.Object) []reconcile.Request {
			vmName, hasLabel := pod.GetLabels()["vm.kubevirt.io/name"]
			if !hasLabel {
				return nil
			}

			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      vmName,
						Namespace: pod.GetNamespace(),
					},
				},
			}
		}),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldPod := e.ObjectOld.(*corev1.Pod)
				newPod := e.ObjectNew.(*corev1.Pod)
				return oldPod.Status.Phase != newPod.Status.Phase
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on Pod: %w", err)
	}

	// Subscribe on VirtualMachineIpAddress.
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachineIPAddress{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			vmip, ok := obj.(*virtv2.VirtualMachineIPAddress)
			if !ok {
				return nil
			}
			name := vmip.Status.VirtualMachine
			if name == "" {
				return nil
			}
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      name,
						Namespace: vmip.GetNamespace(),
					},
				},
			}
		}),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVmip := e.ObjectOld.(*virtv2.VirtualMachineIPAddress)
				newVmip := e.ObjectNew.(*virtv2.VirtualMachineIPAddress)
				return oldVmip.Status.Phase != newVmip.Status.Phase ||
					oldVmip.Status.VirtualMachine != newVmip.Status.VirtualMachine
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachineIpAddress: %w", err)
	}

	// Subscribe on VirtualImage.
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualImage{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsBlockDevice(mgr.GetClient(), virtv2.ImageDevice)),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVi, oldOk := e.ObjectOld.(*virtv2.VirtualImage)
				newVi, newOk := e.ObjectNew.(*virtv2.VirtualImage)
				if !oldOk || !newOk {
					return false
				}
				return oldVi.Status.Phase != newVi.Status.Phase
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualImage: %w", err)
	}

	// Subscribe on VirtualDisk.
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualDisk{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsBlockDevice(mgr.GetClient(), virtv2.DiskDevice)),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldVd, oldOk := e.ObjectOld.(*virtv2.VirtualDisk)
				newVd, newOk := e.ObjectNew.(*virtv2.VirtualDisk)
				if !oldOk || !newOk {
					return false
				}

				oldInUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, oldVd.Status.Conditions)
				newInUseCondition, _ := conditions.GetCondition(vdcondition.InUseType, newVd.Status.Conditions)

				if oldVd.Status.Phase != newVd.Status.Phase || oldInUseCondition != newInUseCondition {
					return true
				}

				return false
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualDisk: %w", err)
	}

	// Subscribe on ClusterVirtualImage.
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.ClusterVirtualImage{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsBlockDevice(mgr.GetClient(), virtv2.ClusterImageDevice)),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldCvi, oldOk := e.ObjectOld.(*virtv2.ClusterVirtualImage)
				newCvi, newOk := e.ObjectNew.(*virtv2.ClusterVirtualImage)
				if !oldOk || !newOk {
					return false
				}
				return oldCvi.Status.Phase != newCvi.Status.Phase
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on ClusterVirtualImage: %w", err)
	}

	for _, w := range []Watcher{
		watcher.NewVirtualMachineClassWatcher(),
		watcher.NewVirtualMachineSnapshotWatcher(mgr.GetClient()),
	} {
		err := w.Watch(mgr, ctr)
		if err != nil {
			return fmt.Errorf("failed to run watcher %s: %w", reflect.TypeOf(w).Elem().Name(), err)
		}
	}

	return nil
}

func (r *Reconciler) enqueueRequestsBlockDevice(cl client.Client, kind virtv2.BlockDeviceKind) func(ctx context.Context, obj client.Object) []reconcile.Request {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		var opts []client.ListOption
		switch kind {
		case virtv2.ImageDevice:
			if _, ok := obj.(*virtv2.VirtualImage); !ok {
				return nil
			}
			opts = append(opts,
				client.InNamespace(obj.GetNamespace()),
				client.MatchingFields{indexer.IndexFieldVMByVI: obj.GetName()},
			)
		case virtv2.ClusterImageDevice:
			if _, ok := obj.(*virtv2.ClusterVirtualImage); !ok {
				return nil
			}
			opts = append(opts,
				client.MatchingFields{indexer.IndexFieldVMByCVI: obj.GetName()},
			)
		case virtv2.DiskDevice:
			if _, ok := obj.(*virtv2.VirtualDisk); !ok {
				return nil
			}
			opts = append(opts,
				client.InNamespace(obj.GetNamespace()),
				client.MatchingFields{indexer.IndexFieldVMByVD: obj.GetName()},
			)
		default:
			return nil
		}
		var vms virtv2.VirtualMachineList
		if err := cl.List(ctx, &vms, opts...); err != nil {
			return nil
		}
		var result []reconcile.Request
		for _, vm := range vms.Items {
			result = append(result, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      vm.GetName(),
					Namespace: vm.GetNamespace(),
				},
			})
		}
		return result
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	vm := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := vm.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if vm.IsEmpty() {
		log.Info("Reconcile observe an absent VirtualMachine: it may be deleted")
		return reconcile.Result{}, nil
	}

	s := state.New(r.client, vm)

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, s)
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		return vm.Update(ctx)
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) factory() *virtv2.VirtualMachine {
	return &virtv2.VirtualMachine{}
}

func (r *Reconciler) statusGetter(obj *virtv2.VirtualMachine) virtv2.VirtualMachineStatus {
	return obj.Status
}
