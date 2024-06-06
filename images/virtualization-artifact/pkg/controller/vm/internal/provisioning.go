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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameProvisioningHandler = "ProvisioningHandler"

func NewProvisioningHandler(client client.Client) *ProvisioningHandler {
	return &ProvisioningHandler{client: client}
}

type ProvisioningHandler struct {
	client client.Client
}

func (h *ProvisioningHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if update := addAllUnknown(changed, string(vmcondition.TypeProvisioningReady)); update {
		return reconcile.Result{Requeue: true}, nil
	}

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	mgr := conditions.NewManager(changed.Status.Conditions)
	cb := conditions.NewConditionBuilder2(vmcondition.TypeProvisioningReady).
		Generation(current.GetGeneration())

	if current.Spec.Provisioning == nil {
		mgr.Update(cb.Status(metav1.ConditionTrue).
			Reason2(vmcondition.ReasonProvisioningReady).
			Message("Provisioning is not defined.").
			Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, nil
	}
	p := current.Spec.Provisioning
	switch p.Type {
	case virtv2.ProvisioningTypeUserData:
		if p.UserData != "" {
			cb.Status(metav1.ConditionTrue).Reason2(vmcondition.ReasonProvisioningReady)
		} else {
			cb.Status(metav1.ConditionFalse).
				Reason2(vmcondition.ReasonProvisioningNotReady).
				Message("Provisioning is defined but it is empty.")
		}
	case virtv2.ProvisioningTypeUserDataRef:
		if p.UserDataRef == nil || p.UserDataRef.Kind != "Secret" {
			cb.Status(metav1.ConditionFalse).
				Reason2(vmcondition.ReasonProvisioningNotReady).
				Message("userdataRef must be \"Secret\"")
		}
		key := types.NamespacedName{Name: p.UserDataRef.Name, Namespace: current.GetNamespace()}
		err := h.genConditionFromSecret(ctx, cb, key)
		if err != nil {
			return reconcile.Result{}, err
		}

	case virtv2.ProvisioningTypeSysprepRef:
		if p.SysprepRef == nil || p.SysprepRef.Kind != "Secret" {
			cb.Status(metav1.ConditionFalse).
				Reason2(vmcondition.ReasonProvisioningNotReady).
				Message("userdataRef must be \"Secret\"")
		}
		key := types.NamespacedName{Name: p.UserDataRef.Name, Namespace: current.GetNamespace()}
		err := h.genConditionFromSecret(ctx, cb, key)
		if err != nil {
			return reconcile.Result{}, err
		}
	default:
		cb.Status(metav1.ConditionFalse).
			Reason2(vmcondition.ReasonProvisioningNotReady).
			Message("Unexpected provisioning type.")
	}

	mgr.Update(cb.Condition())
	changed.Status.Conditions = mgr.Generate()

	return reconcile.Result{}, nil
}

func (h *ProvisioningHandler) Name() string {
	return nameProvisioningHandler
}

func (h *ProvisioningHandler) genConditionFromSecret(ctx context.Context, builder *conditions.ConditionBuilder, secretKey types.NamespacedName) error {
	secret, err := helper.FetchObject(ctx, secretKey, h.client, &corev1.Secret{})
	if err != nil {
		return fmt.Errorf("failed to fetch secret: %w", err)
	}
	if secret == nil {
		builder.Status(metav1.ConditionFalse).
			Reason2(vmcondition.ReasonProvisioningNotReady).
			Message(fmt.Sprintf("Secret %q not found.", secretKey.String()))
		return nil
	}
	if _, ok := secret.Data["userdata"]; !ok {
		builder.Status(metav1.ConditionFalse).
			Reason2(vmcondition.ReasonProvisioningNotReady).
			Message(fmt.Sprintf("Secret %q doesn't have key \"userdata\".", secretKey.String()))
		return nil
	}
	builder.Reason2(vmcondition.ReasonProvisioningReady).Status(metav1.ConditionTrue)
	return nil
}
