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

package step

import (
	"context"
	"errors"
	"fmt"
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
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

type Allocator interface {
	GetAllocatedIPs(ctx context.Context) (ip.AllocatedIPs, error)
	AllocateNewIP(allocatedIPs ip.AllocatedIPs) (string, error)
	IsInsideOfRange(address string) error
}

type CreateLeaseStep struct {
	lease     *virtv2.VirtualMachineIPAddressLease
	allocator Allocator
	client    client.Client
	cb        *conditions.ConditionBuilder
	recorder  eventrecord.EventRecorderLogger
}

func NewCreateLeaseStep(
	lease *virtv2.VirtualMachineIPAddressLease,
	allocator Allocator,
	client client.Client,
	cb *conditions.ConditionBuilder,
	recorder eventrecord.EventRecorderLogger,
) *CreateLeaseStep {
	return &CreateLeaseStep{
		lease:     lease,
		allocator: allocator,
		client:    client,
		cb:        cb,
		recorder:  recorder,
	}
}

func (s CreateLeaseStep) Take(ctx context.Context, vmip *virtv2.VirtualMachineIPAddress) (*reconcile.Result, error) {
	if s.lease != nil {
		err := fmt.Errorf("the VirtualMachineIPAddressLease %q already exists, no need to create a new one, please report this as a bug", vmip.Name)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return nil, err
	}

	// 1. Check if IP address has been already allocated but lost.
	if vmip.Status.Address != "" {
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseLost).
			Message(fmt.Sprintf("The VirtualMachineIPAddressLease %q doesn't exist.", ip.IPToLeaseName(vmip.Status.Address)))
		s.recorder.Event(vmip, corev1.EventTypeWarning, virtv2.ReasonFailed, fmt.Sprintf("The VirtualMachineIPAddressLease %q is lost.", ip.IPToLeaseName(vmip.Status.Address)))
		return &reconcile.Result{}, nil
	}

	allocatedIPs, err := s.allocator.GetAllocatedIPs(ctx)
	if err != nil {
		err = fmt.Errorf("failed to get allcoated IP addresses: %w", err)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return nil, err
	}

	// 2. Allocate a new IP address or use the IP address provided in the spec.
	var ipAddress string
	if vmip.Spec.Type == virtv2.VirtualMachineIPAddressTypeStatic {
		ipAddress = vmip.Spec.StaticIP
	} else {
		ipAddress, err = s.allocator.AllocateNewIP(allocatedIPs)
		if err != nil {
			err = fmt.Errorf("failed to allocate new IP address: %w", err)
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
				Message(service.CapitalizeFirstLetter(err.Error()) + ".")
			return nil, err
		}
	}

	// 3. Verify that the allocated address does not exceed the permissible range.
	err = s.allocator.IsInsideOfRange(ipAddress)
	if err != nil {
		if errors.Is(err, intsvc.ErrIPAddressOutOfRange) {
			msg := fmt.Sprintf("The IP address %q is out of the valid range.", vmip.Spec.StaticIP)
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vmipcondition.VirtualMachineIPAddressIsOutOfTheValidRange).
				Message(msg)
			s.recorder.Event(vmip, corev1.EventTypeWarning, virtv2.ReasonFailed, msg)
			return &reconcile.Result{}, nil
		}

		err = fmt.Errorf("failed to check availability of IP address: %w", err)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return &reconcile.Result{}, err
	}

	// 4. Ensure that the allocated IP address is not represented in the cluster by any lease.
	if _, ok := allocatedIPs[ipAddress]; ok {
		msg := fmt.Sprintf("The IP address %q belongs to existing VirtualMachineIPAddressLease and cannot be taken.", ipAddress)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseAlreadyExists).
			Message(msg)
		s.recorder.Event(vmip, corev1.EventTypeWarning, virtv2.ReasonBound, msg)
		return &reconcile.Result{}, nil
	}

	logger.FromContext(ctx).Info("Create lease", "ipAddress", ipAddress)

	// 5. All checks have passed, create a new lease.
	err = s.client.Create(ctx, buildVirtualMachineIPAddressLease(vmip, ipAddress))
	switch {
	case err == nil:
		msg := fmt.Sprintf("The VirtualMachineIPAddressLease for the ip address %q has been created.", ipAddress)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
			Message(msg)
		s.recorder.Event(vmip, corev1.EventTypeNormal, virtv2.ReasonBound, msg)
		return &reconcile.Result{}, nil
	case k8serrors.IsAlreadyExists(err):
		// The cache is outdated and not keeping up with the state in the cluster.
		// Wait for 2 seconds for the cache to update and try again.
		logger.FromContext(ctx).Warn("Lease already exists: requeue in 2s", "ipAddress", ipAddress)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotFound).
			Message("Waiting for the IP address to be allocated and a new VirtualMachineIPAddressLease to be created.")
		return &reconcile.Result{RequeueAfter: 2 * time.Second}, nil
	default:
		err = fmt.Errorf("failed to create a new VirtualMachineIPAddressLease: %w", err)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotFound).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return nil, err
	}
}

func buildVirtualMachineIPAddressLease(vmip *virtv2.VirtualMachineIPAddress, ipAddress string) *virtv2.VirtualMachineIPAddressLease {
	return &virtv2.VirtualMachineIPAddressLease{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				annotations.LabelVirtualMachineIPAddressUID: string(vmip.GetUID()),
			},
			Name: ip.IPToLeaseName(ipAddress),
		},
		Spec: virtv2.VirtualMachineIPAddressLeaseSpec{
			VirtualMachineIPAddressRef: &virtv2.VirtualMachineIPAddressLeaseIpAddressRef{
				Name:      vmip.Name,
				Namespace: vmip.Namespace,
			},
		},
	}
}
