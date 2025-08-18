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
	"github.com/deckhouse/virtualization-controller/pkg/common/mac"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaccondition"
)

type TakeLeaseStep struct {
	lease    *virtv2.VirtualMachineMACAddressLease
	client   client.Client
	cb       *conditions.ConditionBuilder
	recorder eventrecord.EventRecorderLogger
}

func NewTakeLeaseStep(
	lease *virtv2.VirtualMachineMACAddressLease,
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

func (s TakeLeaseStep) Take(ctx context.Context, vmmac *virtv2.VirtualMachineMACAddress) (*reconcile.Result, error) {
	if s.lease == nil {
		return nil, nil
	}

	vmmacRef := s.lease.Spec.VirtualMachineMACAddressRef

	if vmmacRef != nil && (vmmacRef.Namespace != vmmac.Namespace || vmmacRef.Name != "" || s.lease.Labels[annotations.LabelVirtualMachineMACAddressUID] != "") {
		msg := fmt.Sprintf("The VirtualMachineMACAddressLease %q is already in use by a different virtual machine or belongs to a different namespace.", s.lease.Name)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineMACAddressLeaseNotReady).
			Message(msg)
		s.recorder.Event(vmmac, corev1.EventTypeWarning, vmmaccondition.VirtualMachineMACAddressLeaseAlreadyExists.String(), msg)
		return &reconcile.Result{}, nil
	}

	s.lease.Spec.VirtualMachineMACAddressRef = &virtv2.VirtualMachineMACAddressLeaseMACAddressRef{
		Name:      vmmac.Name,
		Namespace: vmmac.Namespace,
	}
	annotations.AddLabel(s.lease, annotations.LabelVirtualMachineMACAddressUID, string(vmmac.GetUID()))

	err := s.client.Update(ctx, s.lease)
	if err != nil {
		err = fmt.Errorf("failed to update the lease reference %q: %w", mac.LeaseNameToAddress(s.lease.Name), err)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineMACAddressLeaseNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return &reconcile.Result{}, err
	}

	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vmmaccondition.VirtualMachineMACAddressLeaseNotReady).
		Message(fmt.Sprintf("Waiting for the reference of the VirtualMachineMACAddressLease %q to be updated.", s.lease.Name))
	return &reconcile.Result{}, nil
}
