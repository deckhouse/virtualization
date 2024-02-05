package vmop

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/api/v1alpha2"
)

func NewVMOPValidator(log logr.Logger) *VMOPValidator {
	return &VMOPValidator{
		log: log.WithName(vmopControllerName).WithValues("webhook", "validation"),
	}
}

type VMOPValidator struct {
	log logr.Logger
}

func (v *VMOPValidator) ValidateCreate(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *VMOPValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVmop, ok := oldObj.(*v1alpha2.VirtualMachineOperation)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineOperation but got a %T", oldObj)
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

func (v *VMOPValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
