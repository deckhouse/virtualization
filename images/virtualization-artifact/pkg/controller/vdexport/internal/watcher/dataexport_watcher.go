package watcher

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vdexport/dataexport"
)

type DataExportWatcher struct {
	Enabled bool
}

func NewDataExportWatcher(enabled bool) *DataExportWatcher {
	return &DataExportWatcher{
		Enabled: enabled,
	}
}

func (w *DataExportWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if !w.Enabled {
		return nil
	}

	if err := ctr.Watch(source.Kind(mgr.GetCache(), dataexport.NewEmptyDataExport()),
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{},
	); err != nil {
		return fmt.Errorf("error setting watch on DataExport: %w", err)
	}

	return nil
}
