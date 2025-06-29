package watcher

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VDExportWatcher struct{}

func NewVDExportWatcher() *VDExportWatcher {
	return &VDExportWatcher{}
}

func (w *VDExportWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &v1alpha2.VirtualDataExport{}), &handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("error setting watch on VM: %w", err)
	}
	return nil
}
