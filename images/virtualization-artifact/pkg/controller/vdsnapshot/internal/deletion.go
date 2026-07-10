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
	"strings"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const deletionHandlerName = "DeletionHandler"

type DeletionHandler struct {
	snapshotter *service.SnapshotService
}

func NewDeletionHandler(snapshotter *service.SnapshotService) *DeletionHandler {
	return &DeletionHandler{
		snapshotter: snapshotter,
	}
}

func (h DeletionHandler) Handle(ctx context.Context, vdSnapshot *v1alpha2.VirtualDiskSnapshot) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(deletionHandlerName))

	if vdSnapshot.DeletionTimestamp != nil {
		vs, err := h.snapshotter.GetVolumeSnapshot(ctx, vdSnapshot.Status.VolumeSnapshotName, vdSnapshot.Namespace)
		if err != nil {
			return reconcile.Result{}, err
		}

		vd, err := h.snapshotter.GetVirtualDisk(ctx, vdSnapshot.Spec.VirtualDiskName, vdSnapshot.Namespace)
		if err != nil {
			return reconcile.Result{}, err
		}

		var vm *v1alpha2.VirtualMachine
		if vd != nil {
			vm, err = getVirtualMachine(ctx, vd, h.snapshotter)
			if err != nil {
				return reconcile.Result{}, err
			}
		}

		kvvmi, err := h.snapshotter.GetVirtualMachineInstance(ctx, vm)
		if err != nil {
			return reconcile.Result{}, err
		}

		if vs != nil {
			err = h.snapshotter.DeleteVolumeSnapshot(ctx, vs)
			if err != nil {
				return reconcile.Result{}, err
			}
		}

		if vm != nil {
			var canUnfreeze bool
			canUnfreeze, err = h.snapshotter.CanUnfreezeWithVirtualDiskSnapshot(ctx, vdSnapshot.Name, vm, kvvmi)
			if err != nil {
				if errors.Is(err, service.ErrUntrustedFilesystemFrozenCondition) {
					return reconcile.Result{}, nil
				}
				return reconcile.Result{}, err
			}

			// A stopped VM has nothing to unfreeze: the filesystem thaws with the guest.
			if canUnfreeze && kvvmi != nil && kvvmi.Status.Phase == virtv1.Running {
				err = h.snapshotter.Unfreeze(ctx, kvvmi)
				switch {
				case err == nil:
				case k8serrors.IsConflict(err):
					return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
				case strings.Contains(err.Error(), "is not Running"):
					// The VM stopped between the phase check and the unfreeze call.
					log.Info("VirtualMachine is not running anymore, skip unfreeze", "vm.name", vm.Name)
				default:
					return reconcile.Result{}, err
				}
			}
		}

		log.Info("Deletion observed: remove cleanup finalizer from VirtualDiskSnapshot")

		controllerutil.RemoveFinalizer(vdSnapshot, v1alpha2.FinalizerVDSnapshotCleanup)
		return reconcile.Result{}, nil
	}

	controllerutil.AddFinalizer(vdSnapshot, v1alpha2.FinalizerVDSnapshotCleanup)
	return reconcile.Result{}, nil
}
