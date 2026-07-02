//go:build EE
// +build EE

/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package watcher

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachinePoolWatcher struct{}

func NewVirtualMachinePoolWatcher() *VirtualMachinePoolWatcher {
	return &VirtualMachinePoolWatcher{}
}

func (w *VirtualMachinePoolWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(
			mgr.GetCache(),
			&v1alpha2.VirtualMachinePool{},
			&handler.TypedEnqueueRequestForObject[*v1alpha2.VirtualMachinePool]{},
		),
	); err != nil {
		return fmt.Errorf("error setting watch on VirtualMachinePool: %w", err)
	}
	return nil
}
