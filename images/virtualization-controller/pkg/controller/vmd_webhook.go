package controller

import (
	"context"
	"errors"
	"fmt"

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

func (v *VMDValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	vmd, ok := obj.(*v2alpha1.VirtualMachineDisk)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachineDisk but got a %T", obj)
	}

	v.log.Info("Validating VMD", "spec.pvc.size", vmd.Spec.PersistentVolumeClaim.Size)

	if vmd.Spec.PersistentVolumeClaim.Size != nil && vmd.Spec.PersistentVolumeClaim.Size.IsZero() {
		return nil, fmt.Errorf("virtual machine disk size must be greater than 0")
	}

	if vmd.Spec.DataSource == nil && (vmd.Spec.PersistentVolumeClaim.Size == nil || vmd.Spec.PersistentVolumeClaim.Size.IsZero()) {
		return nil, fmt.Errorf("if the data source is not specified, it's necessary to set spec.PersistentVolumeClaim.size to create blank VMD")
	}

	return nil, nil
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

	if newVMD.Spec.PersistentVolumeClaim.Size == nil {
		return nil, errors.New("spec.persistentVolumeClaim.size cannot be omitted once set")
	}

	if newVMD.Spec.PersistentVolumeClaim.Size.IsZero() {
		return nil, fmt.Errorf("virtual machine disk size must be greater than 0")
	}

	if oldVMD.Spec.PersistentVolumeClaim.Size != nil && newVMD.Spec.PersistentVolumeClaim.Size.Cmp(*oldVMD.Spec.PersistentVolumeClaim.Size) == -1 {
		return nil, fmt.Errorf(
			"spec.persistentVolumeClaim.size value (%s) should be greater than or equal to the current value (%s)",
			newVMD.Spec.PersistentVolumeClaim.Size.String(),
			oldVMD.Spec.PersistentVolumeClaim.Size.String(),
		)
	}

	return nil, nil
}

func (v *VMDValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rools: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}
