package vmop

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/api/v1alpha2"
)

func NewValidator(log logr.Logger) *Validator {
	return &Validator{
		log: log.WithName(controllerName).WithValues("webhook", "validation"),
	}
}

type Validator struct {
	log logr.Logger
}

func (v *Validator) ValidateCreate(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rools: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}

func (v *Validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVmop, ok := oldObj.(*v1alpha2.VirtualMachineOperation)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualMachineOperation but got a %T", oldObj)
	}
	newVmop, ok := newObj.(*v1alpha2.VirtualMachineOperation)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineOperation but got a %T", newObj)
	}

	v.log.Info("Validate VMOP updating", "name", oldVmop.GetName())

	if reflect.DeepEqual(oldVmop.Spec, newVmop.Spec) {
		return nil, nil
	}
	err := fmt.Errorf("vmop %q is invalid. vmop.spec is immutable", oldVmop.GetName())
	v.log.Error(err, "Validate VMOP updating", "name", oldVmop.GetName())
	return nil, err
}

func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rools: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}
