package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func NewVMBDAValidator(log logr.Logger) *VMBDAValidator {
	return &VMBDAValidator{log: log}
}

type VMBDAValidator struct {
	log logr.Logger
}

func (v *VMBDAValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	vmbda, ok := obj.(*v1alpha2.VirtualMachineBlockDeviceAttachment)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineBlockDeviceAttachment but got a %T", obj)
	}

	v.log.Info("Validating VMBDA")

	switch vmbda.Spec.BlockDeviceRef.Kind {
	case v1alpha2.VMBDAObjectRefKindVirtualDisk:
		if vmbda.Spec.BlockDeviceRef.Name == "" {
			return nil, errors.New("virtual disk name is omitted, but required")
		}
	default:
		return nil, fmt.Errorf("unknown block device kind %q", vmbda.Spec.BlockDeviceRef.Kind)
	}

	return nil, nil
}

func (v *VMBDAValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newVMBDA, ok := newObj.(*v1alpha2.VirtualMachineBlockDeviceAttachment)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineBlockDeviceAttachment but got a %T", newObj)
	}

	oldVMBDA, ok := oldObj.(*v1alpha2.VirtualMachineBlockDeviceAttachment)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualMachineBlockDeviceAttachment but got a %T", oldObj)
	}

	v.log.Info("Validating VMBDA")

	if newVMBDA.Spec.VirtualMachine != oldVMBDA.Spec.VirtualMachine {
		return nil, errors.New("virtual machine name cannot be changed once set")
	}

	if newVMBDA.Spec.BlockDeviceRef.Kind != oldVMBDA.Spec.BlockDeviceRef.Kind {
		return nil, errors.New("block device type cannot be changed once set")
	}

	switch newVMBDA.Spec.BlockDeviceRef.Kind {
	case v1alpha2.VMBDAObjectRefKindVirtualDisk:
		if newVMBDA.Spec.BlockDeviceRef.Name != oldVMBDA.Spec.BlockDeviceRef.Name {
			return nil, errors.New("virtual disk name cannot be changed once set")
		}
	default:
		return nil, fmt.Errorf("unknown block device kind %q", newVMBDA.Spec.BlockDeviceRef.Kind)
	}

	return nil, nil
}

func (v *VMBDAValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rules: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}
