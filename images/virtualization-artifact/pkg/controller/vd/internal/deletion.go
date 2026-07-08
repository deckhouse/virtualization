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
	"errors"
	"log/slog"
	"sort"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const deletionHandlerName = "DeletionHandler"

type DeletionHandler struct {
	sources *source.Sources
	client  client.Client
}

func NewDeletionHandler(sources *source.Sources, client client.Client) *DeletionHandler {
	return &DeletionHandler{
		sources: sources,
		client:  client,
	}
}

func (h DeletionHandler) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(deletionHandlerName))

	if vd.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(vd, v1alpha2.FinalizerVDProtection) {
			h.setDeletingCondition(
				vd,
				vdcondition.DeletionBlockedByProtection,
				deletionBlockedByProtectionMessage(vd),
			)
			return reconcile.Result{}, nil
		}

		requeue, reason, err := h.sources.CleanUp(ctx, vd)
		if err != nil {
			return reconcile.Result{}, err
		}

		if requeue {
			h.setDeletingCondition(vd, vdcondition.DeletionCleanupPending, reason)
			log.Info("VirtualDisk cleanup is pending", slog.String("reason", reason))
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}

		if err = h.cleanupPersistentVolumeClaims(ctx, vd); err != nil {
			return reconcile.Result{}, err
		}

		conditions.RemoveCondition(vdcondition.DeletingType, &vd.Status.Conditions)
		log.Info("Deletion observed: remove cleanup finalizer from VirtualDisk")
		controllerutil.RemoveFinalizer(vd, v1alpha2.FinalizerVDCleanup)
		return reconcile.Result{}, nil
	}

	conditions.RemoveCondition(vdcondition.DeletingType, &vd.Status.Conditions)
	controllerutil.AddFinalizer(vd, v1alpha2.FinalizerVDCleanup)
	return reconcile.Result{}, nil
}

func (h DeletionHandler) setDeletingCondition(vd *v1alpha2.VirtualDisk, reason vdcondition.DeletingReason, message string) {
	service.SetDeletingCondition(&vd.Status.Conditions, vdcondition.DeletingType, reason, vd.Generation, message)
}

func (h DeletionHandler) cleanupPersistentVolumeClaims(ctx context.Context, vd *v1alpha2.VirtualDisk) error {
	pvcs, err := listPersistentVolumeClaims(ctx, vd, h.client)
	if err != nil {
		return err
	}

	var errs error

	for _, pvc := range pvcs {
		err = deletePersistentVolumeClaim(ctx, &pvc, h.client)
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}

	return errs
}

func deletionBlockedByProtectionMessage(vd *v1alpha2.VirtualDisk) string {
	mountedVMs := make([]string, 0, len(vd.Status.AttachedToVirtualMachines))
	for _, vm := range vd.Status.AttachedToVirtualMachines {
		if vm.Mounted {
			mountedVMs = append(mountedVMs, vm.Name)
		}
	}

	sort.Strings(mountedVMs)

	return service.DeletionBlockedByProtectionMessage("VirtualDisk", mountedVMs)
}
