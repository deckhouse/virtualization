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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type AttacheeHandler struct {
	client client.Client
}

func NewAttacheeHandler(client client.Client) *AttacheeHandler {
	return &AttacheeHandler{
		client: client,
	}
}

func (h AttacheeHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	attachedVMs, err := h.getAttachedVM(ctx, vd)
	if err != nil {
		return reconcile.Result{}, err
	}

	vd.Status.AttachedToVirtualMachines = nil

	for _, vm := range attachedVMs {
		vd.Status.AttachedToVirtualMachines = append(vd.Status.AttachedToVirtualMachines, virtv2.AttachedVirtualMachine{
			Name: vm.Name,
		})
	}

	switch {
	case len(vd.Status.AttachedToVirtualMachines) == 0:
		controllerutil.RemoveFinalizer(vd, virtv2.FinalizerVDProtection)
	case vd.DeletionTimestamp == nil:
		controllerutil.AddFinalizer(vd, virtv2.FinalizerVDProtection)
	}

	return reconcile.Result{}, nil
}

func (h AttacheeHandler) getAttachedVM(ctx context.Context, vd client.Object) ([]virtv2.VirtualMachine, error) {
	var vms virtv2.VirtualMachineList
	err := h.client.List(ctx, &vms, &client.ListOptions{
		Namespace: vd.GetNamespace(),
	})
	if err != nil {
		return nil, fmt.Errorf("error getting virtual machines: %w", err)
	}

	var attachedVMs []virtv2.VirtualMachine

	for _, vm := range vms.Items {
		if h.isVDAttachedToVM(vd.GetName(), vm) {
			attachedVMs = append(attachedVMs, vm)
		}
	}

	return attachedVMs, nil
}

func (h AttacheeHandler) isVDAttachedToVM(vdName string, vm virtv2.VirtualMachine) bool {
	for _, bda := range vm.Status.BlockDeviceRefs {
		if bda.Kind == virtv2.DiskDevice && bda.Name == vdName {
			return true
		}
	}

	return false
}
