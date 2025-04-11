package livemigration

import (
	"context"
	"fmt"

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

	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

type Watcher interface {
	Watch(mgr manager.Manager, ctr controller.Controller) error
}

type Handler interface {
	Handle(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance) (reconcile.Result, error)
	Name() string
}

type Reconciler struct {
	handlers []Handler
	client   client.Client
}

func NewReconciler(client client.Client, handlers ...Handler) *Reconciler {
	return &Reconciler{
		handlers: handlers,
		client:   client,
	}
}

// SetupController adds watchers.
func (r *Reconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	// Subscribe to KVVMI changes.
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &virtv1.VirtualMachineInstance{}), &handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("error setting watch on VM: %w", err)
	}
	// Subscribe to KVMIMigration changes.
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv1.VirtualMachineInstanceMigration{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			kvvmim, ok := obj.(*virtv1.VirtualMachineInstanceMigration)
			if !ok {
				return nil
			}
			vmiName := kvvmim.Spec.VMIName
			if vmiName == "" {
				return nil
			}
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      vmiName,
						Namespace: kvvmim.GetNamespace(),
					},
				},
			}
		}),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldKvvmim := e.ObjectOld.(*virtv1.VirtualMachineInstanceMigration)
				newKvvmim := e.ObjectNew.(*virtv1.VirtualMachineInstanceMigration)
				return oldKvvmim.Status.Phase != newKvvmim.Status.Phase
			},
		},
	); err != nil {
		return fmt.Errorf("error setting watch on VM: %w", err)
	}

	return nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	kvvmi := reconciler.NewResource(req.NamespacedName, r.client, r.factory, r.statusGetter)

	err := kvvmi.Fetch(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvmi.IsEmpty() {
		log.Info("Reconcile observe an absent VirtualMachineInstance: it may be deleted")
		return reconcile.Result{}, nil
	}

	rec := reconciler.NewBaseReconciler[Handler](r.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, h Handler) (reconcile.Result, error) {
		return h.Handle(ctx, kvvmi.Changed())
	})
	rec.SetResourceUpdater(func(ctx context.Context) error {
		// Directly update kvvmi and not use kvvmi.Update as kvvmi status is a regular field, not a subresource.
		if err := r.client.Update(ctx, kvvmi.Changed()); err != nil {
			return fmt.Errorf("error updating status subresource: %w", err)
		}

		return nil
	})

	return rec.Reconcile(ctx)
}

func (r *Reconciler) factory() *virtv1.VirtualMachineInstance {
	return &virtv1.VirtualMachineInstance{}
}

func (r *Reconciler) statusGetter(obj *virtv1.VirtualMachineInstance) virtv1.VirtualMachineInstanceStatus {
	return obj.Status
}
