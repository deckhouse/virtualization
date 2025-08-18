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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/mac"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaccondition"
)

type Allocator interface {
	GetAllocatedAddresses(ctx context.Context) (mac.AllocatedMACs, error)
	AllocateNewAddress(allocatedMACs mac.AllocatedMACs) (string, error)
}

type CreateLeaseStep struct {
	lease     *virtv2.VirtualMachineMACAddressLease
	allocator Allocator
	client    client.Client
	cb        *conditions.ConditionBuilder
	recorder  eventrecord.EventRecorderLogger
}

func NewCreateLeaseStep(
	lease *virtv2.VirtualMachineMACAddressLease,
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

func (s CreateLeaseStep) Take(ctx context.Context, vmmac *virtv2.VirtualMachineMACAddress) (*reconcile.Result, error) {
	if s.lease != nil {
		err := fmt.Errorf("the VirtualMachineMACAddressLease %q already exists, no need to create a new one, please report this as a bug", vmmac.Name)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineMACAddressLeaseNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return nil, err
	}

	// 1. Check if MAC address has been already allocated but lost.
	if vmmac.Status.Address != "" {
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineMACAddressLeaseLost).
			Message(fmt.Sprintf("VirtualMachineMACAddress lost its lease: VirtualMachineMACAddressLease %q should exist", mac.AddressToLeaseName(vmmac.Status.Address)))
		s.recorder.Event(vmmac, corev1.EventTypeWarning, virtv2.ReasonFailed, fmt.Sprintf("The VirtualMachineMACAddressLease %q is lost.", mac.AddressToLeaseName(vmmac.Status.Address)))
		return &reconcile.Result{}, nil
	}

	allocatedAddresses, err := s.allocator.GetAllocatedAddresses(ctx)
	if err != nil {
		err = fmt.Errorf("failed to get allocated MAC addresses: %w", err)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineMACAddressLeaseNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return nil, err
	}

	// 2. Allocate a new MAC address or use the MAC address provided in the spec.
	var macAddress string
	if vmmac.Spec.Address != "" {
		macAddress = vmmac.Spec.Address
	} else {
		macAddress, err = s.allocator.AllocateNewAddress(allocatedAddresses)
		if err != nil {
			err = fmt.Errorf("failed to allocate new MAC address: %w", err)
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vmmaccondition.VirtualMachineMACAddressLeaseNotReady).
				Message(service.CapitalizeFirstLetter(err.Error()) + ".")
			return nil, err
		}
	}

	// 3. Ensure that the chosen MAC address was not already allocated and no lease was taken.
	if _, ok := allocatedAddresses[macAddress]; ok {
		msg := fmt.Sprintf("The MAC address %q belongs to existing VirtualMachineMACAddressLease and cannot be taken.", macAddress)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineMACAddressLeaseAlreadyExists).
			Message(msg)
		s.recorder.Event(vmmac, corev1.EventTypeWarning, virtv2.ReasonBound, msg)
		return &reconcile.Result{}, nil
	}

	logger.FromContext(ctx).Info("Create lease", "macAddress", macAddress)

	// 4. All checks have passed, create a new lease.
	err = s.client.Create(ctx, buildVirtualMachineMACAddressLease(vmmac, macAddress))
	switch {
	case err == nil:
		msg := fmt.Sprintf("The VirtualMachineMACAddressLease for the mac address %q has been created.", macAddress)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineMACAddressLeaseNotReady).
			Message(msg)
		s.recorder.Event(vmmac, corev1.EventTypeNormal, virtv2.ReasonBound, msg)
		return &reconcile.Result{}, nil
	case k8serrors.IsAlreadyExists(err):
		// The cache is outdated and not keeping up with the state in the cluster.
		// Wait for 2 seconds for the cache to update and try again.
		logger.FromContext(ctx).Warn("Lease already exists: requeue in 2s", "macAddress", macAddress)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineMACAddressLeaseNotFound).
			Message("Waiting for the MAC address to be allocated and a new VirtualMachineMACAddressLease to be created.")
		return &reconcile.Result{RequeueAfter: 2 * time.Second}, nil
	default:
		err = fmt.Errorf("failed to create a new VirtualMachineMACAddressLease: %w", err)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineMACAddressLeaseNotFound).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return nil, err
	}
}

func buildVirtualMachineMACAddressLease(vmmac *virtv2.VirtualMachineMACAddress, macAddress string) *virtv2.VirtualMachineMACAddressLease {
	return &virtv2.VirtualMachineMACAddressLease{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				annotations.LabelVirtualMachineMACAddressUID: string(vmmac.GetUID()),
			},
			Name: mac.AddressToLeaseName(macAddress),
		},
		Spec: virtv2.VirtualMachineMACAddressLeaseSpec{
			VirtualMachineMACAddressRef: &virtv2.VirtualMachineMACAddressLeaseMACAddressRef{
				Name:      vmmac.Name,
				Namespace: vmmac.Namespace,
			},
		},
	}
}
