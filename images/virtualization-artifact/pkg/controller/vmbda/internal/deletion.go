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
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const deletionHandlerName = "DeletionHandler"

type DeletionHandler struct{}

func NewDeletionHandler() *DeletionHandler {
	return &DeletionHandler{}
}

func (h DeletionHandler) Handle(ctx context.Context, vd *virtv2.VirtualMachineBlockDeviceAttachment) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(deletionHandlerName))
	if vd.DeletionTimestamp != nil {
		log.Info("Deletion observed: remove cleanup finalizer from VirtualMachineBlockDeviceAttachment")
		controllerutil.RemoveFinalizer(vd, virtv2.FinalizerVMBDACleanup)
		return reconcile.Result{}, nil
	}

	controllerutil.AddFinalizer(vd, virtv2.FinalizerVMBDACleanup)
	return reconcile.Result{}, nil
}
