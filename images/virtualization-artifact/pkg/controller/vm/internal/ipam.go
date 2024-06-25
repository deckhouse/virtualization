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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameIpamHandler = "IPAMHandler"

type IPAM interface {
	IsBound(vmName string, claim *virtv2.VirtualMachineIPAddressClaim) bool
	CheckClaimAvailableForBinding(vmName string, claim *virtv2.VirtualMachineIPAddressClaim) error
	CreateIPAddressClaim(ctx context.Context, vm *virtv2.VirtualMachine, client client.Client) error
	DeleteIPAddressClaim(ctx context.Context, claim *virtv2.VirtualMachineIPAddressClaim, client client.Client) error
}

func NewIPAMHandler(ipam IPAM, cl client.Client, recorder record.EventRecorder, logger *slog.Logger) *IPAMHandler {
	return &IPAMHandler{
		ipam:       ipam,
		client:     cl,
		recorder:   recorder,
		logger:     logger.With("handler", nameIpamHandler),
		protection: service.NewProtectionService(cl, virtv2.FinalizerIPAddressClaimProtection),
	}
}

type IPAMHandler struct {
	ipam       IPAM
	client     client.Client
	recorder   record.EventRecorder
	logger     *slog.Logger
	protection *service.ProtectionService
}

func (h *IPAMHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if update := addAllUnknown(changed, string(vmcondition.TypeIPAddressClaimReady)); update {
		return reconcile.Result{Requeue: true}, nil
	}
	mgr := conditions.NewManager(changed.Status.Conditions)
	cb := conditions.NewConditionBuilder2(vmcondition.TypeIPAddressClaimReady).
		Generation(current.GetGeneration())

	ipAddressClaim, err := s.IPAddressClaim(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if isDeletion(current) {
		return reconcile.Result{}, h.protection.RemoveProtection(ctx, ipAddressClaim)
	}
	err = h.protection.AddProtection(ctx, ipAddressClaim)
	if err != nil {
		return reconcile.Result{}, err
	}

	// 1. OK: already bound.
	if h.ipam.IsBound(current.GetName(), ipAddressClaim) {
		mgr.Update(cb.Status(metav1.ConditionTrue).
			Reason2(vmcondition.ReasonIPAddressClaimReady).
			Condition())
		changed.Status.VirtualMachineIPAddressClaim = ipAddressClaim.GetName()
		changed.Status.IPAddress = ipAddressClaim.Spec.Address
		kvvmi, err := s.KVVMI(ctx)
		if err != nil {
			return reconcile.Result{}, err
		}
		if kvvmi != nil && kvvmi.Status.Phase == virtv1.Running {
			for _, iface := range kvvmi.Status.Interfaces {
				if iface.Name == kvbuilder.NetworkInterfaceName {
					hasClaimedIP := false
					for _, ip := range iface.IPs {
						if ip == ipAddressClaim.Spec.Address {
							hasClaimedIP = true
						}
					}
					if !hasClaimedIP {
						msg := fmt.Sprintf("Claimed IP address (%s) is not among addresses assigned to '%s' network interface (%s)", ipAddressClaim.Spec.Address, kvbuilder.NetworkInterfaceName, strings.Join(iface.IPs, ", "))
						mgr.Update(cb.Status(metav1.ConditionFalse).
							Reason2(vmcondition.ReasonIPAddressClaimNotAssigned).
							Message(msg).
							Condition())
						h.recorder.Event(changed, corev1.EventTypeWarning, vmcondition.ReasonIPAddressClaimNotAssigned.String(), msg)
						h.logger.Error(msg)
					}
					break
				}
			}
		}
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, nil
	}

	cb.Status(metav1.ConditionFalse)

	// 2. Claim not found: create if possible or wait for the claim.
	if ipAddressClaim == nil {
		cb.Reason2(vmcondition.ReasonIPAddressClaimNotReady)
		if current.Spec.VirtualMachineIPAddressClaim != "" {
			h.logger.Info(fmt.Sprintf("The requested ip address claim (%s) for the virtual machine not found: waiting for the Claim", current.Spec.VirtualMachineIPAddressClaim))
			cb.Message(fmt.Sprintf("The requested ip address claim (%s) for the virtual machine not found: waiting for the Claim", current.Spec.VirtualMachineIPAddressClaim))
			return reconcile.Result{RequeueAfter: 2 * time.Second}, nil
		}
		h.logger.Info("VirtualMachineIPAddressClaim not found: create the new one", slog.String("claimName", current.GetName()))
		cb.Message(fmt.Sprintf("VirtualMachineIPAddressClaim %q not found: it may be in the process of being created", current.GetName()))
		mgr.Update(cb.Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{RequeueAfter: 2 * time.Second}, h.ipam.CreateIPAddressClaim(ctx, changed, h.client)
	}

	// 3. Check if possible to bind virtual machine with the found claim.
	err = h.ipam.CheckClaimAvailableForBinding(current.GetName(), ipAddressClaim)
	if err != nil {
		h.logger.Info("Claim is not available to be bound", "err", err, "claimName", current.Spec.VirtualMachineIPAddressClaim)
		reason := vmcondition.ReasonIPAddressClaimNotAvailable.String()
		mgr.Update(cb.Reason(reason).Message(err.Error()).Condition())
		changed.Status.Conditions = mgr.Generate()
		h.recorder.Event(changed, corev1.EventTypeWarning, reason, err.Error())
		return reconcile.Result{}, nil
	}

	// 4. Claim exists and available for binding with virtual machine: waiting for the claim.
	h.logger.Info("Waiting for the Claim to be bound to VM", "claimName", current.Spec.VirtualMachineIPAddressClaim)
	mgr.Update(cb.Reason2(vmcondition.ReasonIPAddressClaimNotReady).
		Message("Claim not bound: waiting for the Claim").Condition())
	changed.Status.Conditions = mgr.Generate()

	return reconcile.Result{RequeueAfter: 2 * time.Second}, nil
}

func (h *IPAMHandler) Name() string {
	return nameIpamHandler
}
