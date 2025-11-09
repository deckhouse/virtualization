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

	commonvm "github.com/deckhouse/virtualization-controller/pkg/common/vm"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type AttacheeHandler struct {
	client client.Client
}

func NewAttacheeHandler(client client.Client) *AttacheeHandler {
	return &AttacheeHandler{
		client: client,
	}
}

func (h AttacheeHandler) Handle(ctx context.Context, vi *v1alpha2.VirtualImage) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("attachee"))

	hasAttachedVM, err := h.hasAttachedVM(ctx, vi)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case !hasAttachedVM:
		log.Debug("Allow virtual image deletion")
		controllerutil.RemoveFinalizer(vi, v1alpha2.FinalizerVIProtection)
	case vi.DeletionTimestamp == nil:
		log.Debug("Protect virtual image from deletion")
		controllerutil.AddFinalizer(vi, v1alpha2.FinalizerVIProtection)
	default:
		log.Debug("Virtual image deletion is delayed: it's protected by virtual machines")
	}

	return reconcile.Result{}, nil
}

func (h AttacheeHandler) Name() string {
	return "AttacheeHandler"
}

func (h AttacheeHandler) hasAttachedVM(ctx context.Context, vi client.Object) (bool, error) {
	var vms v1alpha2.VirtualMachineList
	err := h.client.List(ctx, &vms, &client.ListOptions{
		Namespace: vi.GetNamespace(),
	})
	if err != nil {
		return false, fmt.Errorf("error getting virtual machines: %w", err)
	}

	for _, vm := range vms.Items {
		if vm.Status.Phase == "" {
			continue
		}

		if vm.Status.Phase == v1alpha2.MachineStopped {
			vmIsActive, err := commonvm.IsVMActive(ctx, h.client, vm)
			if err != nil {
				return false, err
			}

			if !vmIsActive {
				continue
			}
		}

		if h.isVIAttachedToVM(vi.GetName(), vm) {
			return true, nil
		}
	}

	return false, nil
}

func (h AttacheeHandler) isVIAttachedToVM(viName string, vm v1alpha2.VirtualMachine) bool {
	for _, bda := range vm.Status.BlockDeviceRefs {
		if bda.Kind == v1alpha2.ImageDevice && bda.Name == viName {
			return true
		}
	}

	return false
}
