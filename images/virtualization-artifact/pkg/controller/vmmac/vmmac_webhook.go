/*
Copyright 2025 Flant JSC

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

package vmmac

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmmac/internal/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaccondition"
)

func NewValidator(
	log *log.Logger,
	client client.Client,
	macAddressService *service.MACAddressService,
) *Validator {
	return &Validator{
		log:        log.With("webhook", "validation"),
		client:     client,
		macService: macAddressService,
	}
}

type Validator struct {
	log        *log.Logger
	client     client.Client
	macService *service.MACAddressService
}

func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	vmmac, ok := obj.(*v1alpha2.VirtualMachineMACAddress)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineMACAddress but got a %T", obj)
	}

	v.log.Info("Validate VirtualMachineMAC creating", "name", vmmac.Name, "address", vmmac.Spec.Address)

	var warnings admission.Warnings

	address := vmmac.Spec.Address
	if address != "" {
		allocatedMACs, err := v.macService.GetAllocatedAddresses(ctx)
		if err != nil {
			return nil, fmt.Errorf("error getting allocated mac addresses: %w", err)
		}

		allocatedLease, ok := allocatedMACs[address]
		if ok && allocatedLease.Spec.VirtualMachineMACAddressRef != nil &&
			(allocatedLease.Spec.VirtualMachineMACAddressRef.Namespace != vmmac.Namespace ||
				allocatedLease.Spec.VirtualMachineMACAddressRef.Name != vmmac.Name) {
			return nil, fmt.Errorf("VirtualMachineMACAddress cannot be created: the MAC address %s has already been allocated by VirtualMachineMACAddress/%s in ns/%s", address, allocatedLease.Spec.VirtualMachineMACAddressRef.Name, allocatedLease.Spec.VirtualMachineMACAddressRef.Namespace)
		}

		err = v.macService.IsAvailableAddress(address, allocatedMACs)
		if err != nil {
			return nil, err
		}
	}

	return warnings, nil
}

func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldMac, ok := oldObj.(*v1alpha2.VirtualMachineMACAddress)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualMachineMAC but got a %T", oldObj)
	}

	newMac, ok := newObj.(*v1alpha2.VirtualMachineMACAddress)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineMAC but got a %T", newObj)
	}

	v.log.Info("Validate VirtualMachineMAC updating", "name", newMac.Name,
		"old.address", oldMac.Spec.Address, "new.address", newMac.Spec.Address,
	)

	boundCondition, exist := conditions.GetCondition(vmmaccondition.BoundType, oldMac.Status.Conditions)
	if exist && boundCondition.Status == metav1.ConditionTrue {
		if oldMac.Spec.Address != newMac.Spec.Address {
			return nil, errors.New("the VirtualMachineMACAddress is in 'Bound' state -> MAC address cannot be changed")
		}
	}

	return nil, nil
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error("Ensure the correctness of ValidatingWebhookConfiguration", "err", err)
	return nil, nil
}
