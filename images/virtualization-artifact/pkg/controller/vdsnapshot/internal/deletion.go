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

const deletionHandlerName = "DeletionHandler"

type DeletionHandler struct {
	snapshotter DeletionSnapshotter
}

func NewDeletionHandler(snapshotter DeletionSnapshotter) *DeletionHandler {
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

		if vs != nil {
			err = h.snapshotter.DeleteVolumeSnapshot(ctx, vs)
			if err != nil {
				return reconcile.Result{}, err
			}
		}

		log.Info("Deletion observed: remove cleanup finalizer from VirtualDiskSnapshot")

		controllerutil.RemoveFinalizer(vdSnapshot, v1alpha2.FinalizerVDSnapshotCleanup)

		return reconcile.Result{}, nil
	}

	controllerutil.AddFinalizer(vdSnapshot, v1alpha2.FinalizerVDSnapshotCleanup)

	return reconcile.Result{}, nil
}
