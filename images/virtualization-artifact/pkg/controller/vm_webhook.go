package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"kubevirt.io/api/core"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VMValidator struct {
	validators []vmValidator
	log        logr.Logger
}

func NewVMValidator(ipam IPAM, client client.Client, log logr.Logger) *VMValidator {
	return &VMValidator{
		validators: []vmValidator{
			newMetaVMValidator(client),
			newIPAMVMValidator(ipam, client),
		},
		log: log.WithName(vmControllerName).WithValues("webhook", "validation"),
	}
}

func (v *VMValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	vm, ok := obj.(*v1alpha2.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachine but got a %T", obj)
	}

	v.log.Info("Validating VM", "spec.virtualMachineIPAddressClaimName", vm.Spec.VirtualMachineIPAddressClaimName)

	var warnings admission.Warnings

	for _, validator := range v.validators {
		warn, err := validator.validateCreate(ctx, vm)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}

	return warnings, nil
}

func (v *VMValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldVM, ok := oldObj.(*v1alpha2.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("expected an old VirtualMachine but got a %T", oldObj)
	}

	newVM, ok := newObj.(*v1alpha2.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("expected a new VirtualMachine but got a %T", newObj)
	}

	v.log.Info("Validating VM",
		"old.spec.virtualMachineIPAddressClaimName", oldVM.Spec.VirtualMachineIPAddressClaimName,
		"new.spec.virtualMachineIPAddressClaimName", newVM.Spec.VirtualMachineIPAddressClaimName,
	)

	var warnings admission.Warnings

	for _, validator := range v.validators {
		warn, err := validator.validateUpdate(ctx, oldVM, newVM)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}

	return warnings, nil
}

func (v *VMValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	err := fmt.Errorf("misconfigured webhook rools: delete operation not implemented")
	v.log.Error(err, "Ensure the correctness of ValidatingWebhookConfiguration")
	return nil, nil
}

type vmValidator interface {
	validateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error)
	validateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error)
}

type metaVMValidator struct {
	client client.Client
}

func newMetaVMValidator(client client.Client) *metaVMValidator {
	return &metaVMValidator{client: client}
}

func (v *metaVMValidator) validateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	for key := range vm.Annotations {
		if strings.Contains(key, core.GroupName) {
			return nil, fmt.Errorf("using the %s group's name in the annotation is prohibited", core.GroupName)
		}
	}

	for key := range vm.Labels {
		if strings.Contains(key, core.GroupName) {
			return nil, fmt.Errorf("using the %s group's name in the label is prohibited", core.GroupName)
		}
	}

	return nil, nil
}

func (v *metaVMValidator) validateUpdate(_ context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	for key := range newVM.Annotations {
		if strings.Contains(key, core.GroupName) {
			return nil, fmt.Errorf("using the %s group's name in the annotation is prohibited", core.GroupName)
		}
	}

	for key := range newVM.Labels {
		if strings.Contains(key, core.GroupName) {
			return nil, fmt.Errorf("using the %s group's name in the label is prohibited", core.GroupName)
		}
	}

	return nil, nil
}

type ipamVMValidator struct {
	ipam   IPAM
	client client.Client
}

func newIPAMVMValidator(ipam IPAM, client client.Client) *ipamVMValidator {
	return &ipamVMValidator{ipam: ipam, client: client}
}

func (v *ipamVMValidator) validateCreate(ctx context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	claimName := vm.Spec.VirtualMachineIPAddressClaimName
	if claimName == "" {
		claimName = vm.Name
	}

	claimKey := types.NamespacedName{Name: claimName, Namespace: vm.Namespace}
	claim, err := helper.FetchObject(ctx, claimKey, v.client, &v1alpha2.VirtualMachineIPAddressClaim{})
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

func (v *ipamVMValidator) validateUpdate(ctx context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if oldVM.Spec.VirtualMachineIPAddressClaimName == newVM.Spec.VirtualMachineIPAddressClaimName {
		return nil, nil
	}

	if newVM.Spec.VirtualMachineIPAddressClaimName == "" {
		return nil, fmt.Errorf("spec.virtualMachineIPAddressClaimName cannot be changed to an empty value once set")
	}

	claimKey := types.NamespacedName{Name: newVM.Spec.VirtualMachineIPAddressClaimName, Namespace: newVM.Namespace}
	claim, err := helper.FetchObject(ctx, claimKey, v.client, &v1alpha2.VirtualMachineIPAddressClaim{})
	if err != nil {
		return nil, fmt.Errorf("unable to get VirtualMachineIPAddressClaim %s: %w", claimKey, err)
	}

	if claim == nil {
		return nil, nil
	}

	return nil, v.ipam.CheckClaimAvailableForBinding(newVM.Name, claim)
}
