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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

type TakeLeaseStep struct {
	lease    *v1alpha2.VirtualMachineIPAddressLease
	client   client.Client
	cb       *conditions.ConditionBuilder
	recorder eventrecord.EventRecorderLogger
}

func NewTakeLeaseStep(
	lease *v1alpha2.VirtualMachineIPAddressLease,
	client client.Client,
	cb *conditions.ConditionBuilder,
	recorder eventrecord.EventRecorderLogger,
) *TakeLeaseStep {
	return &TakeLeaseStep{
		lease:    lease,
		client:   client,
		cb:       cb,
		recorder: recorder,
	}
}

func (s TakeLeaseStep) Take(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress) (*reconcile.Result, error) {
	if s.lease == nil {
		return nil, nil
	}

	// 1. A VirtualMachineIPAddress can only use the Lease that has the same namespace ref.
	// It cannot override the namespace ref of a Lease for itself.
	vmipRef := s.lease.Spec.VirtualMachineIPAddressRef
	if vmipRef != nil && vmipRef.Namespace != "" && vmipRef.Namespace != vmip.Namespace {
		msg := fmt.Sprintf("The VirtualMachineIPAddressLease %q belongs to a different namespace.", s.lease.Name)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
			Message(msg)
		s.recorder.Event(vmip, corev1.EventTypeWarning, vmipcondition.VirtualMachineIPAddressLeaseAlreadyExists.String(), msg)
		return &reconcile.Result{}, nil
	}

	// 2. Ensure that the Lease is not occupied by another vmip.
	if vmipRef != nil && vmipRef.Name != "" || s.lease.Labels[annotations.LabelVirtualMachineIPAddressUID] != "" {
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
			Message(fmt.Sprintf("The VirtualMachineIPAddressLease %q already has a reference to another VirtualMachineIPAddress.", s.lease.Name))
		return &reconcile.Result{}, nil
	}

	// All checks have passed, the Lease is unoccupied, and it can be taken.
	s.lease.Spec.VirtualMachineIPAddressRef = &v1alpha2.VirtualMachineIPAddressLeaseIpAddressRef{
		Name:      vmip.Name,
		Namespace: vmip.Namespace,
	}
	annotations.AddLabel(s.lease, annotations.LabelVirtualMachineIPAddressUID, string(vmip.GetUID()))

	err := s.client.Update(ctx, s.lease)
	if err != nil {
		err = fmt.Errorf("failed to update the lease reference %q: %w", ip.LeaseNameToIP(s.lease.Name), err)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return &reconcile.Result{}, err
	}

	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
		Message(fmt.Sprintf("Waiting for the reference of the VirtualMachineIPAddressLease %q to be updated.", s.lease.Name))
	return &reconcile.Result{}, nil
}
