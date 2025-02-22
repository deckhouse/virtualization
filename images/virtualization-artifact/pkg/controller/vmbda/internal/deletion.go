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
	"log/slog"
	"strings"

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
	CanUnplug(kvvm *virtv1.VirtualMachine, diskName string) bool
	UnplugDisk(ctx context.Context, kvvm *virtv1.VirtualMachine, diskName string) error
}
type DeletionHandler struct {
	unplug UnplugInterface
	client client.Client

	log *slog.Logger
}

func NewDeletionHandler(unplug UnplugInterface, client client.Client) *DeletionHandler {
	return &DeletionHandler{
		unplug: unplug,
		client: client,
	}
}

func (h *DeletionHandler) Handle(ctx context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (reconcile.Result, error) {
	h.log = logger.FromContext(ctx).With(logger.SlogHandler(deletionHandlerName))

	if vmbda.DeletionTimestamp != nil {
		if err := h.cleanUp(ctx, vmbda); err != nil {
			return reconcile.Result{}, err
		}
		h.log.Info("Deletion observed: remove cleanup finalizer from VirtualMachineBlockDeviceAttachment")
		controllerutil.RemoveFinalizer(vmbda, virtv2.FinalizerVMBDACleanup)
		return reconcile.Result{}, nil
	}

	controllerutil.AddFinalizer(vmbda, virtv2.FinalizerVMBDACleanup)
	return reconcile.Result{}, nil
}

func (h *DeletionHandler) cleanUp(ctx context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) error {
	if vmbda == nil {
		return nil
	}

	var diskName string
	switch vmbda.Spec.BlockDeviceRef.Kind {
	case virtv2.VMBDAObjectRefKindVirtualDisk:
		diskName = kvbuilder.GenerateVMDDiskName(vmbda.Spec.BlockDeviceRef.Name)
	case virtv2.VMBDAObjectRefKindVirtualImage:
		diskName = kvbuilder.GenerateVMIDiskName(vmbda.Spec.BlockDeviceRef.Name)
	case virtv2.VMBDAObjectRefKindClusterVirtualImage:
		diskName = kvbuilder.GenerateCVMIDiskName(vmbda.Spec.BlockDeviceRef.Name)
	}

	kvvm, err := object.FetchObject(ctx, types.NamespacedName{Namespace: vmbda.GetNamespace(), Name: vmbda.Spec.VirtualMachineName}, h.client, &virtv1.VirtualMachine{})
	if err != nil {
		return err
	}

	if h.unplug.CanUnplug(kvvm, diskName) {
		h.log.Info("Unplug Virtual Disk", slog.String("diskName", diskName), slog.String("vm", kvvm.Name))
		if err = h.unplug.UnplugDisk(ctx, kvvm, diskName); err != nil {
			if strings.Contains(err.Error(), "does not exist") {
				return nil
			}
			return err
		}
	}
	return nil
}
