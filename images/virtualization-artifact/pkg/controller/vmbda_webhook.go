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

	switch vmbda.Spec.BlockDevice.Type {
	case v1alpha2.BlockDeviceAttachmentTypeVirtualMachineDisk:
		if vmbda.Spec.BlockDevice.VirtualMachineDisk == nil || vmbda.Spec.BlockDevice.VirtualMachineDisk.Name == "" {
			return nil, errors.New("virtual machine disk name is omitted, but required")
		}
	default:
		return nil, fmt.Errorf("unknown block device type %q", vmbda.Spec.BlockDevice.Type)
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

	if newVMBDA.Spec.VMName != oldVMBDA.Spec.VMName {
		return nil, errors.New("virtual machine name cannot be changed once set")
	}

	if newVMBDA.Spec.BlockDevice.Type != oldVMBDA.Spec.BlockDevice.Type {
		return nil, errors.New("block device type cannot be changed once set")
	}

	switch newVMBDA.Spec.BlockDevice.Type {
	case v1alpha2.BlockDeviceAttachmentTypeVirtualMachineDisk:
		if newVMBDA.Spec.BlockDevice.VirtualMachineDisk == nil || newVMBDA.Spec.BlockDevice.VirtualMachineDisk.Name != oldVMBDA.Spec.BlockDevice.VirtualMachineDisk.Name {
			return nil, errors.New("virtual machine disk name cannot be changed once set")
		}
	default:
		return nil, fmt.Errorf("unknown block device type %q", newVMBDA.Spec.BlockDevice.Type)
	}

	return nil, nil
}

func (v *VMBDAValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rools: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}
