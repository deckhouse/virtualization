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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/mac"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmmac/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmmac/internal/util"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaccondition"
)

const MACLeaseHandlerName = "MACLeaseHandler"

type MACLeaseHandler struct {
	client     client.Client
	macService *service.MACAddressService
	recorder   record.EventRecorder
}

func NewMACLeaseHandler(client client.Client, macAddressService *service.MACAddressService, recorder record.EventRecorder) *MACLeaseHandler {
	return &MACLeaseHandler{
		client:     client,
		macService: macAddressService,
		recorder:   recorder,
	}
}

func (h MACLeaseHandler) Handle(ctx context.Context, state state.VMMACState) (reconcile.Result, error) {
	log, ctx := logger.GetHandlerContext(ctx, MACLeaseHandlerName)

	vmmac := state.VirtualMachineMAC()
	macStatus := &vmmac.Status

	lease, err := state.VirtualMachineMACLease(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	condition, _ := conditions.GetCondition(vmmaccondition.BoundType, macStatus.Conditions)

	switch {
	case lease == nil && macStatus.Address != "" && condition.Reason != vmmaccondition.VirtualMachineMACAddressLeaseAlreadyExists.String():
		log.Info("Lease by name not found: waiting for the lease to be available")
		return reconcile.Result{}, nil

	case lease == nil:
		log.Info("No Lease for VirtualMachineMAC: create the new one", "address", vmmac.Spec.Address)
		return h.createNewLease(ctx, state)

	case lease.Status.Phase == "":
		log.Info("Lease is not ready: waiting for the lease")
		return reconcile.Result{}, nil

	case util.IsBoundLease(lease, vmmac):
		log.Info("Lease already exists, VirtualMachineMAC ref is valid")
		return reconcile.Result{}, nil

	case lease.Status.Phase == virtv2.VirtualMachineMACAddressLeasePhaseBound:
		log.Info("Lease is bounded to another VirtualMachineMAC: recreate VirtualMachineMAC when the lease is released")
		return reconcile.Result{}, nil

	default:
		log.Info("Lease is released: set binding")

		if lease.Spec.VirtualMachineMACAddressRef.Namespace != vmmac.Namespace {
			log.Warn(fmt.Sprintf("The VirtualMachineMACLease belongs to a different namespace: %s", lease.Spec.VirtualMachineMACAddressRef.Namespace))
			h.recorder.Event(vmmac, corev1.EventTypeWarning, vmmaccondition.VirtualMachineMACAddressLeaseAlreadyExists.String(), "The VirtualMachineMACLease belongs to a different namespace")

			return reconcile.Result{}, nil
		}

		lease.Spec.VirtualMachineMACAddressRef = &virtv2.VirtualMachineMACAddressLeaseMACAddressRef{
			Name:      vmmac.Name,
			Namespace: vmmac.Namespace,
		}

		err := h.client.Update(ctx, lease)
		if err != nil {
			return reconcile.Result{}, err
		}

		macStatus.Address = mac.LeaseNameToAddress(lease.Name)
		return reconcile.Result{}, nil
	}
}

func (h MACLeaseHandler) createNewLease(ctx context.Context, state state.VMMACState) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	vmmac := state.VirtualMachineMAC()
	macStatus := &vmmac.Status

	if vmmac.Spec.Address == "" {
		log.Info("allocate the new VirtualMachineMAC address")
		var err error
		macStatus.Address, err = h.macService.AllocateNewAddress(state.AllocatedMACs())
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		macStatus.Address = vmmac.Spec.Address
	}

	err := h.macService.IsAvailableAddress(macStatus.Address, state.AllocatedMACs())
	if err != nil {
		macStatus.Address = ""
		msg := fmt.Sprintf("the VirtualMachineMAC cannot be created: %s", err.Error())
		log.Info(msg)

		conditionBound := conditions.NewConditionBuilder(vmmaccondition.BoundType).
			Generation(vmmac.GetGeneration())

		switch {
		case errors.Is(err, service.ErrMACAddressOutOfRange):
			macStatus.Phase = virtv2.VirtualMachineMACAddressPhasePending
			conditionBound.Status(metav1.ConditionFalse).
				Reason(vmmaccondition.VirtualMachineMACAddressIsOutOfTheValidRange).
				Message(fmt.Sprintf("The requested MAC address %s is out of the valid range",
					vmmac.Spec.Address))
			h.recorder.Event(vmmac, corev1.EventTypeWarning, vmmaccondition.VirtualMachineMACAddressIsOutOfTheValidRange.String(), msg)
		case errors.Is(err, service.ErrMACAddressAlreadyExist):
			macStatus.Phase = virtv2.VirtualMachineMACAddressPhasePending
			conditionBound.Status(metav1.ConditionFalse).
				Reason(vmmaccondition.VirtualMachineMACAddressLeaseAlreadyExists).
				Message(fmt.Sprintf("VirtualMachineMACAddressLease %s is bound to another VirtualMachineMACAddress",
					mac.AddressToLeaseName(macStatus.Address)))
			h.recorder.Event(vmmac, corev1.EventTypeWarning, vmmaccondition.VirtualMachineMACAddressLeaseAlreadyExists.String(), msg)
		}
		conditions.SetCondition(conditionBound, &macStatus.Conditions)
		return reconcile.Result{}, nil
	}

	leaseName := mac.AddressToLeaseName(macStatus.Address)

	log.Info("Create lease",
		"leaseName", leaseName,
		"refName", vmmac.Name,
		"refNamespace", vmmac.Namespace,
	)

	err = h.client.Create(ctx, &virtv2.VirtualMachineMACAddressLease{
		ObjectMeta: metav1.ObjectMeta{
			Name: leaseName,
		},
		Spec: virtv2.VirtualMachineMACAddressLeaseSpec{
			VirtualMachineMACAddressRef: &virtv2.VirtualMachineMACAddressLeaseMACAddressRef{
				Name:      vmmac.Name,
				Namespace: vmmac.Namespace,
			},
		},
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (h MACLeaseHandler) Name() string {
	return MACLeaseHandlerName
}
