package controller

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/deckhouse/virtualization-controller/pkg/controller/ipam"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	vmmetrics "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/virtualmachine"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	vmControllerName = "vm-controller"
)

func NewVMController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
	dvcrSettings *dvcr.Settings,
) (controller.Controller, error) {
	reconciler := &VMReconciler{
		dvcrSettings: dvcrSettings,
		ipam:         ipam.New(),
	}
	mgrCache := mgr.GetCache()
	reconcilerCore := two_phase_reconciler.NewReconcilerCore[*VMReconcilerState](
		reconciler,
		NewVMReconcilerState,
		two_phase_reconciler.ReconcilerOptions{
			Client:   mgr.GetClient(),
			Cache:    mgrCache,
			Recorder: mgr.GetEventRecorderFor(vmControllerName),
			Scheme:   mgr.GetScheme(),
			Log:      log.WithName(vmControllerName),
		})

	c, err := controller.New(vmControllerName, mgr, controller.Options{Reconciler: reconcilerCore})
	if err != nil {
		return nil, err
	}

	if err = reconciler.SetupController(ctx, mgr, c); err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachine{}).
		WithValidator(NewVMValidator(ipam.New(), mgr.GetClient(), log)).
		Complete(); err != nil {
		return nil, err
	}

	vmmetrics.SetupCollector(&vmLister{vmCache: mgrCache}, metrics.Registry)

	log.Info("Initialized VirtualMachine controller")
	return c, nil
}

type vmLister struct {
	vmCache cache.Cache
}

func (l vmLister) List() ([]v1alpha2.VirtualMachine, error) {
	vmList := v1alpha2.VirtualMachineList{}
	err := l.vmCache.List(context.Background(), &vmList)
	if err != nil {
		return nil, err
	}
	return vmList.Items, nil
}
