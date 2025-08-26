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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/mac"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vmmac/internal/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaccondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaclcondition"
)

type BindStep struct {
	lease *virtv2.VirtualMachineMACAddressLease
	cb    *conditions.ConditionBuilder
}

func NewBindStep(
	lease *virtv2.VirtualMachineMACAddressLease,
	cb *conditions.ConditionBuilder,
) *BindStep {
	return &BindStep{
		lease: lease,
		cb:    cb,
	}
}

func (s BindStep) Take(_ context.Context, vmmac *virtv2.VirtualMachineMACAddress) (*reconcile.Result, error) {
	// 1. The required Lease already exists; set its address in the vmmac status.
	if s.lease != nil {
		vmmac.Status.Address = mac.LeaseNameToAddress(s.lease.Name)
	}

	// 2. The vmmac can be Bound only if the assigned Lease has a fully populated and matching reference.
	if !intsvc.HasReference(vmmac, s.lease) {
		return nil, nil
	}

	// 3. A vmmac can become bound only if the corresponding Lease is bound as well.
	boundCondition, _ := conditions.GetCondition(vmmaclcondition.BoundType, s.lease.Status.Conditions)
	if boundCondition.Status != metav1.ConditionTrue || !conditions.IsLastUpdated(boundCondition, s.lease) {
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmmaccondition.VirtualMachineMACAddressLeaseNotReady).
			Message(fmt.Sprintf("Waiting for the VirtualMachineMACAddressLease %q to be bound.", s.lease.Name))
		return &reconcile.Result{}, nil
	}

	// 4. All checks have passed, the vmmac is bound.
	s.cb.
		Status(metav1.ConditionTrue).
		Reason(vmmaccondition.Bound).
		Message("")
	return &reconcile.Result{}, nil
}
