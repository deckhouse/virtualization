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

package internal

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/mac"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmmaclcondition"
)

const LifecycleHandlerName = "LifecycleHandler"

type LifecycleHandler struct {
	client   client.Client
	recorder eventrecord.EventRecorderLogger
}

func NewLifecycleHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *LifecycleHandler {
	return &LifecycleHandler{
		client:   client,
		recorder: recorder,
	}
}

func (h *LifecycleHandler) Handle(ctx context.Context, lease *virtv2.VirtualMachineMACAddressLease) (reconcile.Result, error) {
	cb := conditions.NewConditionBuilder(vmmaclcondition.BoundType).Generation(lease.GetGeneration())

	vmmacKey := types.NamespacedName{Name: lease.Spec.VirtualMachineMACAddressRef.Name, Namespace: lease.Spec.VirtualMachineMACAddressRef.Namespace}
	vmmac, err := object.FetchObject(ctx, vmmacKey, h.client, &virtv2.VirtualMachineMACAddress{})
	if err != nil {
		cb.
			Status(metav1.ConditionUnknown).
			Reason(conditions.ReasonUnknown).
			Message(fmt.Sprintf("Failed to fetch VirtualMachineMACAddress: %s.", err))
		conditions.SetCondition(cb, &lease.Status.Conditions)
		return reconcile.Result{}, fmt.Errorf("fetch vmmac %s: %w", vmmacKey, err)
	}

	// Lease is Bound, if there is a vmmac with matched Ref.
	if isBound(lease, vmmac) {
		annotations.AddLabel(lease, annotations.LabelVirtualMachineMACAddressUID, string(vmmac.UID))
		if lease.Status.Phase != virtv2.VirtualMachineMACAddressLeasePhaseBound {
			h.recorder.Eventf(lease, corev1.EventTypeNormal, virtv2.ReasonBound, "VirtualMachineMACAddressLease is bound to \"%s/%s\".", vmmac.Namespace, vmmac.Name)
		}
		lease.Status.Phase = virtv2.VirtualMachineMACAddressLeasePhaseBound
		cb.
			Status(metav1.ConditionTrue).
			Reason(vmmaclcondition.Bound).
			Message("")
		conditions.SetCondition(cb, &lease.Status.Conditions)
	} else {
		cb.
			Status(metav1.ConditionFalse).
			Reason(conditions.ReasonUnknown).
			Message("VirtualMachineMACAddressLease is not bound to any VirtualMachineMACAddress.")
		conditions.SetCondition(cb, &lease.Status.Conditions)
	}

	return reconcile.Result{}, nil
}

func (h *LifecycleHandler) Name() string {
	return LifecycleHandlerName
}

func isBound(lease *virtv2.VirtualMachineMACAddressLease, vmmac *virtv2.VirtualMachineMACAddress) bool {
	if lease == nil || vmmac == nil {
		return false
	}

	vmmacRef := lease.Spec.VirtualMachineMACAddressRef
	if vmmacRef == nil || vmmac.Name != vmmacRef.Name || vmmac.Namespace != vmmacRef.Namespace {
		return false
	}

	if vmmac.Status.Address != "" && vmmac.Status.Address != mac.LeaseNameToAddress(lease.Name) {
		return false
	}

	return true
}
