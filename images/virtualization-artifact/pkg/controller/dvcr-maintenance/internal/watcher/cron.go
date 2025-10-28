package watcher

import (
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/virtualization-controller/pkg/controller/gc"
)

type CronWatcher struct {
}

func NewCronWatcher() *CronWatcher {
	return &CronWatcher{}
}

func (w *CronWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	cronSource := gc.NewCronSource()
	return ctr.Watch(cronSource)
}
