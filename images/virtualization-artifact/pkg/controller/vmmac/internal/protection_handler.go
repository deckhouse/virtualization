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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmmac/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const ProtectionHandlerName = "ProtectionHandler"

type ProtectionHandler struct {
	client client.Client
}

func NewProtectionHandler(client client.Client) *ProtectionHandler {
	return &ProtectionHandler{
		client: client,
	}
}

func (h *ProtectionHandler) Handle(ctx context.Context, state state.VMMACState) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(ProtectionHandlerName))

	mac := state.VirtualMachineMAC()

	vm, err := state.VirtualMachine(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	configuredVms, err := h.getConfiguredVM(ctx, mac)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case len(configuredVms) == 0:
		log.Debug("Allow VirtualMachineMACAddress deletion: remove protection finalizer")
		controllerutil.RemoveFinalizer(mac, virtv2.FinalizerMACAddressProtection)
	case mac.DeletionTimestamp == nil:
		log.Debug("Protect VirtualMachineMACAddress from deletion")
		controllerutil.AddFinalizer(mac, virtv2.FinalizerMACAddressProtection)
	default:
		log.Debug("VirtualMachineMACAddress deletion is delayed: it's protected by virtual machines")
	}

	if vm == nil || vm.DeletionTimestamp != nil {
		log.Info("VirtualMachineMAC is no longer attached to any VM: remove cleanup finalizer", "VirtualMachineMACName", mac.Name)
		controllerutil.RemoveFinalizer(mac, virtv2.FinalizerMACAddressCleanup)
	} else if mac.GetDeletionTimestamp() == nil {
		controllerutil.AddFinalizer(mac, virtv2.FinalizerMACAddressCleanup)
		log.Info("VirtualMachineMAC is still attached, finalizer added", "VirtualMachineMACName", mac.Name)
	}

	return reconcile.Result{}, nil
}

func (h *ProtectionHandler) Name() string {
	return ProtectionHandlerName
}

func (h *ProtectionHandler) getConfiguredVM(ctx context.Context, vmmac *virtv2.VirtualMachineMACAddress) ([]virtv2.VirtualMachine, error) {
	var vms virtv2.VirtualMachineList
	err := h.client.List(ctx, &vms, &client.ListOptions{
		Namespace: vmmac.Namespace,
	})
	if err != nil {
		return nil, err
	}

	var configuredVms []virtv2.VirtualMachine
	for _, vm := range vms.Items {
		if vm.Spec.VirtualMachineMACAddress == vmmac.Name && vm.Status.Phase != virtv2.MachineTerminating {
			configuredVms = append(configuredVms, vm)
		}
	}

	return configuredVms, nil
}
