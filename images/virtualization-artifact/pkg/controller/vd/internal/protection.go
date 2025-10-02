/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package internal

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ProtectionHandler struct{}

func NewProtectionHandler() *ProtectionHandler {
	return &ProtectionHandler{}
}

func (h ProtectionHandler) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("protection"))

	if len(vd.Status.AttachedToVirtualMachines) > 1 {
		log.Debug("virtual disk connected to multiple virtual machines", "vms", len(vd.Status.AttachedToVirtualMachines))
	}

	unmounted := true
	for _, vm := range vd.Status.AttachedToVirtualMachines {
		if vm.Mounted {
			unmounted = false
			break
		}
	}

	if unmounted || vd.Status.Phase == v1alpha2.DiskPending {
		log.Debug("Allow virtual disk deletion")
		controllerutil.RemoveFinalizer(vd, v1alpha2.FinalizerVDProtection)
		return reconcile.Result{}, nil
	}

	if vd.DeletionTimestamp == nil {
		log.Debug("Protect virtual disk from deletion")
		controllerutil.AddFinalizer(vd, v1alpha2.FinalizerVDProtection)
	}
	return reconcile.Result{}, nil
}
