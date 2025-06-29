package vdexport

import (
	"context"
	"time"

	"github.com/deckhouse/deckhouse/pkg/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vdexport/internal/handler"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const (
	ControllerName = "vdexport-controller"
)

func SetupController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
	dataExportEnabled bool,
	exporterImage string,
	requirements corev1.ResourceRequirements,
	dvcr *dvcr.Settings,
	controllerNamespace string,
) error {
	client := mgr.GetClient()

	sourceCreator := handler.NewExportSourceCreator(exporterImage, requirements, dvcr, controllerNamespace)

	handlers := []Handler{
		handler.NewDeletionHandler(client, dataExportEnabled, sourceCreator),
		handler.NewLifecycleHandler(client, dataExportEnabled, sourceCreator),
	}
	reconciler := NewReconciler(client, dataExportEnabled, handlers...)

	vdExportController, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       reconciler,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
	})
	if err != nil {
		return err
	}

	err = reconciler.SetupController(ctx, mgr, vdExportController)
	if err != nil {
		return err
	}

	log.Info("Initialized VirtualDataExport controller")
	return nil
}
