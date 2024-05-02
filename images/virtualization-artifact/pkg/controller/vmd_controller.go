package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	diskmetrics "github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/virtualdisk"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	vmdControllerName = "vd-controller"
	vmdShortName      = "vd"
)

func NewVMDController(
	ctx context.Context,
	mgr manager.Manager,
	log logr.Logger,
	importerImage string,
	uploaderImage string,
	dvcrSettings *dvcr.Settings,
) (controller.Controller, error) {
	reconciler := NewVMDReconciler(
		importerImage,
		uploaderImage,
		ImporterPodVerbose,
		ImporterPodPullPolicy,
		dvcrSettings,
	)
	mgrCache := mgr.GetCache()
	reconcilerCore := two_phase_reconciler.NewReconcilerCore[*VMDReconcilerState](
		reconciler,
		NewVMDReconcilerState,
		two_phase_reconciler.ReconcilerOptions{
			Client:   mgr.GetClient(),
			Cache:    mgrCache,
			Recorder: mgr.GetEventRecorderFor(vmdControllerName),
			Scheme:   mgr.GetScheme(),
			Log:      log.WithName(vmdControllerName),
		})

	c, err := controller.New(vmdControllerName, mgr, controller.Options{
		Reconciler:  reconcilerCore,
		RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(time.Second, 32*time.Second),
	})
	if err != nil {
		return nil, err
	}

	if err = reconciler.SetupController(ctx, mgr, c); err != nil {
		return nil, err
	}

	if err = builder.WebhookManagedBy(mgr).
		For(&v1alpha2.VirtualDisk{}).
		WithValidator(NewVMDValidator(log)).
		Complete(); err != nil {
		return nil, err
	}

	diskmetrics.SetupCollector(&diskLister{diskCache: mgrCache}, metrics.Registry)

	log.Info("Initialized VirtualDisk controller")
	return c, nil
}

type diskLister struct {
	diskCache cache.Cache
}

func (l diskLister) List() ([]v1alpha2.VirtualDisk, error) {
	disks := v1alpha2.VirtualDiskList{}
	err := l.diskCache.List(context.Background(), &disks)
	if err != nil {
		return nil, err
	}
	return disks.Items, nil
}
