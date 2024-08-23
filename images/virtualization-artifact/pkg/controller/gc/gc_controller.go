package gc

import (
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

func SetupGcController(
	controllerName string,
	mgr manager.Manager,
	log *slog.Logger,
	watchSource source.Source,
	newObject NewObject,
	isNeedDelete IsNeedDelete,
) error {
	log = log.With(logger.SlogController(controllerName))
	recorder := mgr.GetEventRecorderFor(controllerName)
	reconciler := NewReconciler(mgr.GetClient(),
		recorder,
		watchSource,
		newObject,
		isNeedDelete,
	)

	err := reconciler.SetupWithManager(controllerName, mgr, log)
	if err != nil {
		return err
	}

	log.Info("Initialized garbage collector controller")

	return nil
}
