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
	"log/slog"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/util"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

const IpLeaseHandlerName = "IPLeaseHandler"

type IPLeaseHandler struct {
	client    client.Client
	ipService *service.IpAddressService
	recorder  eventrecord.EventRecorderLogger
}

func NewIPLeaseHandler(client client.Client, ipAddressService *service.IpAddressService, recorder eventrecord.EventRecorderLogger) *IPLeaseHandler {
	return &IPLeaseHandler{
		client:    client,
		ipService: ipAddressService,
		recorder:  recorder,
	}
}

func (h IPLeaseHandler) Handle(ctx context.Context, state state.VMIPState) (reconcile.Result, error) {
	log, ctx := logger.GetHandlerContext(ctx, IpLeaseHandlerName)

	vmip, lease := state.VirtualMachineIP(), state.VirtualMachineIPLease()

	switch {
	case lease == nil:
		log.Info("No Lease for VirtualMachineIP: create the new one", "type", vmip.Spec.Type, "address", vmip.Spec.StaticIP)
		return h.createNewLease(ctx, state)

	case lease.Status.Phase == "":
		log.Info("Lease is not ready: waiting for the lease")
		return reconcile.Result{}, nil

	case vmip.Status.Address == "":
		vmip.Status.Address = ip.LeaseNameToIP(lease.Name)
		return reconcile.Result{}, nil

	case util.IsBoundLease(lease, vmip):
		log.Info("Lease is bound, VirtualMachineIP ref is valid")
		return reconcile.Result{}, nil

	case lease.Status.Phase == virtv2.VirtualMachineIPAddressLeasePhaseBound:
		log.Info("Lease is bounded to another VirtualMachineIP: recreate VirtualMachineIP when the lease is released")
		return reconcile.Result{}, nil

	default:
		return h.updateLease(ctx, lease, vmip, log)
	}
}

func (h IPLeaseHandler) updateLease(ctx context.Context, lease *virtv2.VirtualMachineIPAddressLease, vmip *virtv2.VirtualMachineIPAddress, log *slog.Logger) (reconcile.Result, error) {
	log.Info("Lease is released: set binding")

	if lease.Spec.VirtualMachineIPAddressRef.Namespace != vmip.Namespace {
		log.Warn(fmt.Sprintf("The VirtualMachineIPLease belongs to a different namespace: %s", lease.Spec.VirtualMachineIPAddressRef.Namespace))
		h.recorder.Event(vmip, corev1.EventTypeWarning, vmipcondition.VirtualMachineIPAddressLeaseAlreadyExists.String(), "The VirtualMachineIPLease belongs to a different namespace")

		return reconcile.Result{}, nil
	}

	lease.Spec.VirtualMachineIPAddressRef = &virtv2.VirtualMachineIPAddressLeaseIpAddressRef{
		Name:      vmip.Name,
		Namespace: vmip.Namespace,
	}
	lease.Labels[annotations.LabelVirtualMachineIPAddressUID] = string(vmip.GetUID())

	return reconcile.Result{}, h.client.Update(ctx, lease)
}

func (h IPLeaseHandler) createNewLease(ctx context.Context, state state.VMIPState) (reconcile.Result, error) {
	log := logger.FromContext(ctx)

	vmip := state.VirtualMachineIP()
	ipAddress := ""
	if vmip.Spec.Type == virtv2.VirtualMachineIPAddressTypeAuto {
		log.Info("allocate the new VirtualMachineIP address")
		var err error
		ipAddress, err = h.ipService.AllocateNewIP(state.AllocatedIPs())
		if err != nil {
			return reconcile.Result{}, err
		}
	} else {
		ipAddress = vmip.Spec.StaticIP
	}

	err := h.ipService.IsAvailableAddress(ipAddress, state.AllocatedIPs())
	if err != nil {
		msg := fmt.Sprintf("the VirtualMachineIP cannot be created: %s", err.Error())
		log.Warn(msg)

		conditionBound := conditions.NewConditionBuilder(vmipcondition.BoundType).
			Generation(vmip.GetGeneration())

		switch {
		case errors.Is(err, service.ErrIPAddressOutOfRange):
			vmip.Status.Phase = virtv2.VirtualMachineIPAddressPhasePending
			msg = fmt.Sprintf("The requested address %s is out of the valid range.", vmip.Spec.StaticIP)
			conditionBound.Status(metav1.ConditionFalse).
				Reason(vmipcondition.VirtualMachineIPAddressIsOutOfTheValidRange).
				Message(msg)
			h.recorder.Event(vmip, corev1.EventTypeWarning, virtv2.ReasonFailed, msg)
		case errors.Is(err, service.ErrIPAddressAlreadyExist):
			vmip.Status.Phase = virtv2.VirtualMachineIPAddressPhasePending
			msg = fmt.Sprintf("VirtualMachineIPAddressLease %s is bound to another VirtualMachineIPAddress.",
				ip.IpToLeaseName(ipAddress))
			conditionBound.Status(metav1.ConditionFalse).
				Reason(vmipcondition.VirtualMachineIPAddressLeaseAlreadyExists).
				Message(msg)
			h.recorder.Event(vmip, corev1.EventTypeWarning, virtv2.ReasonBound, msg)
		}
		conditions.SetCondition(conditionBound, &vmip.Status.Conditions)
		return reconcile.Result{}, nil
	}

	leaseName := ip.IpToLeaseName(ipAddress)

	log.Info("Create lease", "leaseName", leaseName,
		"refName", vmip.Name, "refNamespace", vmip.Namespace,
	)

	err = h.client.Create(ctx, &virtv2.VirtualMachineIPAddressLease{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				annotations.LabelVirtualMachineIPAddressUID: string(vmip.GetUID()),
			},
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
		if k8serrors.IsAlreadyExists(err) {
			log.Warn("Lease already exists: requeue 2s")
			return reconcile.Result{RequeueAfter: 2 * time.Second}, nil
		}

		return reconcile.Result{}, err
	}

	h.recorder.Event(vmip, corev1.EventTypeNormal, virtv2.ReasonBound, "VirtualMachineIPAddress is bound to a new VirtualMachineIPAddressLease.")

	return reconcile.Result{}, nil
}

func (h IPLeaseHandler) Name() string {
	return IpLeaseHandlerName
}
