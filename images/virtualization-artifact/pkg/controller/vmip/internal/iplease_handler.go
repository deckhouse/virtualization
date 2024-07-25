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
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

const IpLeaseHandlerName = "IPLeaseHandler"

type IPLeaseHandler struct {
	client    client.Client
	logger    logr.Logger
	ipService *service.IpAddressService
	recorder  record.EventRecorder
}

func NewIPLeaseHandler(client client.Client, logger logr.Logger, ipAddressService *service.IpAddressService, recorder record.EventRecorder) *IPLeaseHandler {
	return &IPLeaseHandler{
		client:    client,
		logger:    logger.WithValues("handler", IpLeaseHandlerName),
		ipService: ipAddressService,
		recorder:  recorder,
	}
}

func (h IPLeaseHandler) Handle(ctx context.Context, state state.VMIPState) (reconcile.Result, error) {
	vmip := state.VirtualMachineIP()
	vmipStatus := &vmip.Status

	lease, err := state.VirtualMachineIPLease(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case lease == nil && vmipStatus.Address != "":
		h.logger.Info("Lease by name not found: waiting for the lease to be available")
		return reconcile.Result{}, nil

	case lease == nil:
		h.logger.Info("No Lease for VirtualMachineIP: create the new one", "type", vmip.Spec.Type, "address", vmip.Spec.StaticIP)
		return h.createNewLease(ctx, state)

	case lease.Status.Phase == "":
		// TODO: Replace this requeue with proper UpdateFunc in VirtualMachineIPAddressLease watch setup.
		h.logger.Info("Lease is not ready: waiting for the lease")
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, nil

	case util.IsBoundLease(lease, vmip):
		h.logger.Info("Lease already exists, VirtualMachineIP ref is valid")
		return reconcile.Result{}, nil

	case lease.Status.Phase == virtv2.VirtualMachineIPAddressLeasePhaseBound:
		h.logger.Info("Lease is bounded to another VirtualMachineIP: recreate VirtualMachineIP when the lease is released")
		return reconcile.Result{}, nil

	default:
		h.logger.Info("Lease is released: set binding")

		if lease.Spec.VirtualMachineIPAddressRef.Namespace != vmip.Namespace {
			return reconcile.Result{}, fmt.Errorf("the selected VirtualMachineIP lease belongs to a different namespace: %s", lease.Spec.VirtualMachineIPAddressRef.Namespace)
		}

		lease.Spec.VirtualMachineIPAddressRef = &virtv2.VirtualMachineIPAddressLeaseIpAddressRef{
			Name:      vmip.Name,
			Namespace: vmip.Namespace,
		}

		err := h.client.Update(ctx, lease)
		if err != nil {
			return reconcile.Result{}, err
		}

		vmipStatus.Address = common.LeaseNameToIP(lease.Name)
		return reconcile.Result{}, nil
	}
}

func (h IPLeaseHandler) createNewLease(ctx context.Context, state state.VMIPState) (reconcile.Result, error) {
	vmip := state.VirtualMachineIP()
	vmipStatus := &vmip.Status

	if vmip.Spec.Type == virtv2.VirtualMachineIPAddressTypeAuto {
		h.logger.Info("allocate the new VirtualMachineIP address")
		var err error
		vmipStatus.Address, err = h.ipService.AllocateNewIP(state.AllocatedIPs())
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		vmipStatus.Address = vmip.Spec.StaticIP
	}

	err := h.ipService.IsAvailableAddress(vmipStatus.Address, state.AllocatedIPs())
	if err != nil {
		vmipStatus.Address = ""
		msg := fmt.Sprintf("the VirtualMachineIP cannot be created: %s", err.Error())
		h.logger.Info(msg)

		mgr := conditions.NewManager(vmipStatus.Conditions)
		conditionBound := conditions.NewConditionBuilder(vmipcondition.BoundType).
			Generation(vmip.GetGeneration())

		switch {
		case errors.Is(err, service.ErrIPAddressOutOfRange):
			vmipStatus.Phase = virtv2.VirtualMachineIPAddressPhasePending
			mgr.Update(conditionBound.Status(metav1.ConditionFalse).
				Reason(vmipcondition.VirtualMachineIPAddressIsOutOfTheValidRange).
				Message(fmt.Sprintf("The requested address %s is out of the valid range",
					vmip.Spec.StaticIP)).
				Condition())
			h.recorder.Event(vmip, corev1.EventTypeWarning, vmipcondition.VirtualMachineIPAddressIsOutOfTheValidRange, msg)
		case errors.Is(err, service.ErrIPAddressAlreadyExist):
			vmipStatus.Phase = virtv2.VirtualMachineIPAddressPhasePending
			mgr.Update(conditionBound.Status(metav1.ConditionFalse).
				Reason(vmipcondition.VirtualMachineIPAddressLeaseAlready).
				Message(fmt.Sprintf("VirtualMachineIPAddressLease %s is bound to another VirtualMachineIP",
					common.IpToLeaseName(vmipStatus.Address))).
				Condition())
			h.recorder.Event(vmip, corev1.EventTypeWarning, vmipcondition.VirtualMachineIPAddressLeaseAlready, msg)
		}

		vmipStatus.Conditions = mgr.Generate()
		return reconcile.Result{}, err
	}

	leaseName := common.IpToLeaseName(vmipStatus.Address)

	h.logger.Info("Create lease",
		"leaseName", leaseName,
		"refName", vmip.Name,
		"refNamespace", vmip.Namespace,
	)

	err = h.client.Create(ctx, &virtv2.VirtualMachineIPAddressLease{
		ObjectMeta: metav1.ObjectMeta{
			Name: leaseName,
		},
		Spec: virtv2.VirtualMachineIPAddressLeaseSpec{
			VirtualMachineIPAddressRef: &virtv2.VirtualMachineIPAddressLeaseIpAddressRef{
				Name:      vmip.Name,
				Namespace: vmip.Namespace,
			},
		},
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (h IPLeaseHandler) Name() string {
	return IpLeaseHandlerName
}
