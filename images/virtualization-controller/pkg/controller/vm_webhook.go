package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

type IPAMValidator interface {
	CheckClaimAvailableForBinding(vmName string, claim *v2alpha1.VirtualMachineIPAddressClaim) error
}

func NewVMValidator(ipam IPAMValidator, client client.Client, log logr.Logger) *VMValidator {
	return &VMValidator{
		ipam:   ipam,
		client: client,
		log:    log.WithName(vmControllerName).WithValues("webhook", "validation"),
	}
}

type VMValidator struct {
	ipam   IPAMValidator
	client client.Client
	log    logr.Logger
}

func (v *VMValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	vm, ok := obj.(*v2alpha1.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachine but got a %T", obj)
	}

	v.log.Info("Validating VM", "spec.virtualMachineIPAddressClaimName", vm.Spec.VirtualMachineIPAddressClaimName)

	claimName := vm.Spec.VirtualMachineIPAddressClaimName
	if claimName == "" {
		claimName = vm.Name
	}

	claimKey := types.NamespacedName{Name: claimName, Namespace: vm.Namespace}
	claim, err := helper.FetchObject(ctx, claimKey, v.client, &v2alpha1.VirtualMachineIPAddressClaim{})
	if err != nil {
		return nil, fmt.Errorf("unable to get Claim %s: %w", claimKey, err)
	}

	if claim == nil {
		return nil, nil
	}

	if vm.Spec.VirtualMachineIPAddressClaimName == "" {
		return nil, fmt.Errorf("VirtualMachineIPAddressClaim with the name of the virtual machine"+
			" already exists: explicitly specify the name of the VirtualMachineIPAddressClaim (%s)"+
			" in spec.virtualMachineIPAddressClaimName of virtual machine", claim.Name)
	}

	return nil, v.ipam.CheckClaimAvailableForBinding(vm.Name, claim)
}

func (v *VMValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVM, ok := oldObj.(*v2alpha1.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualMachine but got a %T", oldObj)
	}

	newVM, ok := newObj.(*v2alpha1.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachine but got a %T", newObj)
	}

	v.log.Info("Validating VM",
		"old.spec.virtualMachineIPAddressClaimName", oldVM.Spec.VirtualMachineIPAddressClaimName,
		"new.spec.virtualMachineIPAddressClaimName", newVM.Spec.VirtualMachineIPAddressClaimName,
	)

	if oldVM.Spec.VirtualMachineIPAddressClaimName == newVM.Spec.VirtualMachineIPAddressClaimName {
		return nil, nil
	}

	if newVM.Spec.VirtualMachineIPAddressClaimName == "" {
		return nil, fmt.Errorf("spec.virtualMachineIPAddressClaimName cannot be changed to an empty value once set")
	}

	claimKey := types.NamespacedName{Name: newVM.Spec.VirtualMachineIPAddressClaimName, Namespace: newVM.Namespace}
	claim, err := helper.FetchObject(ctx, claimKey, v.client, &v2alpha1.VirtualMachineIPAddressClaim{})
	if err != nil {
		return nil, fmt.Errorf("unable to get VirtualMachineIPAddressClaim %s: %w", claimKey, err)
	}

	if claim == nil {
		return nil, nil
	}

	return nil, v.ipam.CheckClaimAvailableForBinding(newVM.Name, claim)
}

func (v *VMValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rools: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}
