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
	"fmt"
	"log/slog"
	"time"

	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const deletionHandlerName = "DeletionHandler"

type UnplugInterface interface {
	IsAttached(vm *virtv2.VirtualMachine, kvvm *virtv1.VirtualMachine, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) bool
	UnplugDisk(ctx context.Context, kvvm *virtv1.VirtualMachine, diskName string) error
}
type DeletionHandler struct {
	unplug UnplugInterface
	client client.Client
}

func NewDeletionHandler(unplug UnplugInterface, client client.Client) *DeletionHandler {
	return &DeletionHandler{
		unplug: unplug,
		client: client,
	}
}

func (h *DeletionHandler) Handle(ctx context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (reconcile.Result, error) {
	controllerutil.AddFinalizer(vmbda, virtv2.FinalizerVMBDACleanup)

	if vmbda.DeletionTimestamp == nil {
		return reconcile.Result{}, nil
	}

	vm, err := object.FetchObject(ctx, types.NamespacedName{Namespace: vmbda.GetNamespace(), Name: vmbda.Spec.VirtualMachineName}, h.client, &virtv2.VirtualMachine{})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch vm: %w", err)
	}

	kvvm, err := object.FetchObject(ctx, types.NamespacedName{Namespace: vmbda.GetNamespace(), Name: vmbda.Spec.VirtualMachineName}, h.client, &virtv1.VirtualMachine{})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch intvirtvm: %w", err)
	}

	if h.unplug.IsAttached(vm, kvvm, vmbda) {
		var res reconcile.Result
		res, err = h.detach(ctx, kvvm, vmbda)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to detach: %w", err)
		}
		return res, nil
	}

	log := logger.FromContext(ctx).With(logger.SlogHandler(deletionHandlerName))
	log.Info("Deletion observed: remove cleanup finalizer from VirtualMachineBlockDeviceAttachment")
	controllerutil.RemoveFinalizer(vmbda, virtv2.FinalizerVMBDACleanup)
	return reconcile.Result{}, nil
}

func (h *DeletionHandler) detach(ctx context.Context, kvvm *virtv1.VirtualMachine, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (reconcile.Result, error) {
	var blockDeviceName string
	switch vmbda.Spec.BlockDeviceRef.Kind {
	case virtv2.VMBDAObjectRefKindVirtualDisk:
		blockDeviceName = kvbuilder.GenerateVMDDiskName(vmbda.Spec.BlockDeviceRef.Name)
	case virtv2.VMBDAObjectRefKindVirtualImage:
		blockDeviceName = kvbuilder.GenerateVMIDiskName(vmbda.Spec.BlockDeviceRef.Name)
	case virtv2.VMBDAObjectRefKindClusterVirtualImage:
		blockDeviceName = kvbuilder.GenerateCVMIDiskName(vmbda.Spec.BlockDeviceRef.Name)
	}

	log := logger.FromContext(ctx).With(logger.SlogHandler(deletionHandlerName))
	if kvvm != nil {
		log.Info("Unplug block device", slog.String("blockDeviceName", blockDeviceName), slog.String("vm", kvvm.Name))
	}
	err := h.unplug.UnplugDisk(ctx, kvvm, blockDeviceName)
	if err != nil {
		switch {
		case IsDoesNotExistError(err):
			return reconcile.Result{}, nil
		case IsOutdatedRequestError(err):
			log.Debug("The server rejected our request, retry")
			return reconcile.Result{RequeueAfter: time.Second}, nil
		}

		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
