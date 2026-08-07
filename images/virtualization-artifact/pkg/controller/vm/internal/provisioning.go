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
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/cloudinit"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const nameProvisioningHandler = "ProvisioningHandler"

// eventReasonProvisioningInvalid reports malformed provisioning data.
const eventReasonProvisioningInvalid = "ProvisioningInvalid"

func NewProvisioningHandler(client client.Client, recorder eventrecord.EventRecorderLogger) *ProvisioningHandler {
	return &ProvisioningHandler{client: client, recorder: recorder, validator: newProvisioningValidator(client)}
}

type ProvisioningHandler struct {
	client    client.Client
	recorder  eventrecord.EventRecorderLogger
	validator *provisioningValidator
}

func (h *ProvisioningHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}

	current := s.VirtualMachine().Current()
	changed := s.VirtualMachine().Changed()

	if update := addAllUnknown(changed, vmcondition.TypeProvisioningReady); update {
		return reconcile.Result{Requeue: true}, nil
	}

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	cb := conditions.NewConditionBuilder(vmcondition.TypeProvisioningReady).
		Generation(current.GetGeneration())

	if current.Spec.Provisioning == nil {
		conditions.SetCondition(cb.Status(metav1.ConditionTrue).
			Reason(vmcondition.ReasonProvisioningReady), &changed.Status.Conditions)
		return reconcile.Result{}, nil
	}
	p := current.Spec.Provisioning
	var warnings []string
	switch p.Type {
	case v1alpha2.ProvisioningTypeUserData:
		if p.UserData != "" {
			warnings = cloudinit.ValidateUserData([]byte(p.UserData))
			cb.Status(metav1.ConditionTrue).Reason(vmcondition.ReasonProvisioningReady)
		} else {
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonProvisioningNotReady).
				Message("Provisioning is defined but it is empty.")
		}
	case v1alpha2.ProvisioningTypeUserDataRef:
		if p.UserDataRef == nil || p.UserDataRef.Kind != v1alpha2.UserDataRefKindSecret {
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonProvisioningNotReady).
				Message(fmt.Sprintf("spec.provisioning.userDataRef.kind must be %q.", v1alpha2.UserDataRefKindSecret))
			break
		}
		key := types.NamespacedName{Name: p.UserDataRef.Name, Namespace: current.GetNamespace()}
		secretWarnings, err := h.genConditionFromSecret(ctx, cb, key)
		if err != nil {
			return reconcile.Result{}, err
		}
		warnings = secretWarnings

	case v1alpha2.ProvisioningTypeSysprepRef:
		if p.SysprepRef == nil || p.SysprepRef.Kind != v1alpha2.SysprepRefKindSecret {
			cb.Status(metav1.ConditionFalse).
				Reason(vmcondition.ReasonProvisioningNotReady).
				Message(fmt.Sprintf("spec.provisioning.sysprepRef.kind must be %q.", v1alpha2.SysprepRefKindSecret))
			break
		}
		key := types.NamespacedName{Name: p.SysprepRef.Name, Namespace: current.GetNamespace()}
		secretWarnings, err := h.genConditionFromSecret(ctx, cb, key)
		if err != nil {
			return reconcile.Result{}, err
		}
		warnings = secretWarnings
	default:
		cb.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonProvisioningNotReady).
			Message("Unexpected provisioning type.")
	}

	if len(warnings) > 0 {
		message := service.CapitalizeFirstLetter(strings.Join(warnings, "; ")) + "."
		cb.Message(message)
		// The settings are still usable, so the condition stays true and the
		// virtual machine starts; a reason of its own keeps that apart from a
		// provisioning nothing is wrong with.
		if cb.Condition().Status == metav1.ConditionTrue {
			cb.Reason(vmcondition.ReasonProvisioningReadyWithWarnings)
		}
		h.recorder.Event(current, corev1.EventTypeWarning, eventReasonProvisioningInvalid, message)
	}

	conditions.SetCondition(cb, &changed.Status.Conditions)

	return reconcile.Result{}, nil
}

func (h *ProvisioningHandler) Name() string {
	return nameProvisioningHandler
}

// genConditionFromSecret sets the condition from the referenced secret and
// returns warnings about its contents.
func (h *ProvisioningHandler) genConditionFromSecret(ctx context.Context, builder *conditions.ConditionBuilder, secretKey types.NamespacedName) ([]string, error) {
	warnings, err := h.validator.Validate(ctx, secretKey)

	switch {
	case err == nil:
		builder.Reason(vmcondition.ReasonProvisioningReady).Status(metav1.ConditionTrue)
		return warnings, nil
	case errors.As(err, new(secretNotFoundError)):
		builder.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonProvisioningNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return nil, nil

	case errors.Is(err, errSecretIsNotValid):
		builder.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonProvisioningNotReady).
			Message(fmt.Sprintf("Invalid secret %q: %s", secretKey.String(), err.Error()))
		return nil, nil

	case errors.As(err, new(unexpectedSecretTypeError)):
		builder.Status(metav1.ConditionFalse).
			Reason(vmcondition.ReasonProvisioningNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()))
		return nil, nil

	default:
		return nil, err
	}
}

var errSecretIsNotValid = errors.New("secret is not valid")

type secretNotFoundError string

func (e secretNotFoundError) Error() string {
	return fmt.Sprintf("secret %s not found", string(e))
}

type unexpectedSecretTypeError string

func (e unexpectedSecretTypeError) Error() string {
	return fmt.Sprintf("unexpected secret type: %s", string(e))
}

var cloudInitCheckKeys = []string{
	"userdata",
	"userData",
}

// sysprepCheckKeys are the answer files Windows setup looks for on the sysprep
// disk. See https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/wsim/windows-system-image-manager-technical-reference
var sysprepCheckKeys = []string{
	"autounattend.xml",
	"unattend.xml",
}

func newProvisioningValidator(reader client.Reader) *provisioningValidator {
	return &provisioningValidator{
		reader: reader,
	}
}

type provisioningValidator struct {
	reader client.Reader
}

// Validate reports whether the secret can be used for provisioning at all, and
// separately what looks wrong with the data it carries. An error means the
// reference itself is unusable; warnings describe malformed contents.
func (v provisioningValidator) Validate(ctx context.Context, key types.NamespacedName) ([]string, error) {
	secret := &corev1.Secret{}
	err := v.reader.Get(ctx, key, secret)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, secretNotFoundError(key.String())
		}
		return nil, err
	}
	switch secret.Type {
	case v1alpha2.SecretTypeCloudInit:
		return v.validateCloudInitSecret(secret)
	case v1alpha2.SecretTypeSysprep:
		return v.validateSysprepSecret(secret)
	default:
		return nil, unexpectedSecretTypeError(secret.Type)
	}
}

func (v provisioningValidator) validateCloudInitSecret(secret *corev1.Secret) ([]string, error) {
	key, found := v.findKey(secret, cloudInitCheckKeys...)
	if !found {
		return nil, fmt.Errorf("the secret should have one of data fields %v: %w", cloudInitCheckKeys, errSecretIsNotValid)
	}
	return cloudinit.ValidateUserData(secret.Data[key]), nil
}

func (v provisioningValidator) validateSysprepSecret(secret *corev1.Secret) ([]string, error) {
	if _, found := v.findKey(secret, sysprepCheckKeys...); !found {
		return []string{fmt.Sprintf("the secret has none of the data fields %v, so Windows setup will find no answer file", sysprepCheckKeys)}, nil
	}
	return nil, nil
}

// findKey returns the first of checkKeys the secret carries.
func (v provisioningValidator) findKey(secret *corev1.Secret, checkKeys ...string) (string, bool) {
	for _, key := range checkKeys {
		if _, ok := secret.Data[key]; ok {
			return key, true
		}
	}
	return "", false
}
