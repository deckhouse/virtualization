package controller

import (
	"context"
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

func NewVMDValidator(log logr.Logger) *VMDValidator {
	return &VMDValidator{log: log}
}

type VMDValidator struct {
	log logr.Logger
}

func (v *VMDValidator) ValidateCreate(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	// TODO check spec.pvc.size format uses binary prefixes (Mi, Gi).
	// Use resource.ParseQuantity to validate or change type of size field to resource.Quantity.

	panic("not implemented")
}

func (v *VMDValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	newVMD, ok := newObj.(*v2alpha1.VirtualMachineDisk)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineDisk but got a %T", newObj)
	}

	oldVMD, ok := oldObj.(*v2alpha1.VirtualMachineDisk)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualMachineDisk but got a %T", oldObj)
	}

	v.log.Info("Validating VMD",
		"old.spec.pvc.size", oldVMD.Spec.PersistentVolumeClaim.Size,
		"new.spec.pvc.size", newVMD.Spec.PersistentVolumeClaim.Size,
	)

	if newVMD.Spec.PersistentVolumeClaim.Size == oldVMD.Spec.PersistentVolumeClaim.Size {
		return nil, nil
	}

	newPVCSize, err := humanize.ParseBytes(newVMD.Spec.PersistentVolumeClaim.Size)
	if err != nil {
		return nil, err
	}

	oldPVCSize, err := humanize.ParseBytes(oldVMD.Spec.PersistentVolumeClaim.Size)
	if err != nil {
		return nil, err
	}

	if newPVCSize < oldPVCSize {
		return nil, fmt.Errorf("new value of spec.persistentVolumeClaim.size (%s) cannot be smaller than the old one (%s)", newVMD.Spec.PersistentVolumeClaim.Size, oldVMD.Spec.PersistentVolumeClaim.Size)
	}

	return nil, nil
}

func (v *VMDValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	panic("not implemented")
}
