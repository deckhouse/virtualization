package controller

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	vmbdametrics "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/vmbda"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const VMBDAControllerName = "vmbda-controller"

func NewVMBDAController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
	controllerNamespace string,
) (controller.Controller, error) {
	reconciler := NewVMBDAReconciler(controllerNamespace)

	mgrCache := mgr.GetCache()
	reconcilerCore := two_phase_reconciler.NewReconcilerCore[*VMBDAReconcilerState](
		reconciler,
		NewVMBDAReconcilerState,
		two_phase_reconciler.ReconcilerOptions{
			Client:   mgr.GetClient(),
			Cache:    mgrCache,
			Recorder: mgr.GetEventRecorderFor(VMBDAControllerName),
			Scheme:   mgr.GetScheme(),
			Log:      log.WithName(VMBDAControllerName),
		})

	cvmiController, err := controller.New(VMBDAControllerName, mgr, controller.Options{Reconciler: reconcilerCore})
	if err != nil {
		return nil, err
	}
	if err = reconciler.SetupController(ctx, mgr, cvmiController); err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualMachineBlockDeviceAttachment{}).
		WithValidator(NewVMBDAValidator(log)).
		Complete(); err != nil {
		return nil, err
	}

	vmbdametrics.SetupCollector(&vmbdaLister{vmbdaCache: mgrCache}, metrics.Registry)

	log.Info("Initialized VirtualMachineBlockDeviceAttachment controller")
	return cvmiController, nil
}

type vmbdaLister struct {
	vmbdaCache cache.Cache
}

func (l vmbdaLister) List() ([]v1alpha2.VirtualMachineBlockDeviceAttachment, error) {
	vmbdas := v1alpha2.VirtualMachineBlockDeviceAttachmentList{}
	err := l.vmbdaCache.List(context.Background(), &vmbdas)
	if err != nil {
		return nil, err
	}
	return vmbdas.Items, nil
}
