package defaulter

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineClassNameDefaulter struct {
	client         client.Client
	vmClassService *service.VirtualMachineClassService
}

func NewVirtualMachineClassNameDefaulter(client client.Client, vmClassService *service.VirtualMachineClassService) *VirtualMachineClassNameDefaulter {
	return &VirtualMachineClassNameDefaulter{
		client:         client,
		vmClassService: vmClassService,
	}
}

func (v *VirtualMachineClassNameDefaulter) Default(ctx context.Context, vm *v1alpha2.VirtualMachine) error {
	classes := &v1alpha2.VirtualMachineClassList{}
	err := v.client.List(ctx, classes)
	if err != nil {
		return fmt.Errorf("failed to list virtual machine classes: %w", err)
	}

	defaultClass, err := v.vmClassService.GetDefault(classes)
	if err != nil {
		return err
	}

	// "No default class" is not a mutating error, validators will complain
	// about missing field during validation phase later.
	if defaultClass == nil {
		return nil
	}

	vm.Spec.VirtualMachineClassName = defaultClass.GetName()
	return nil
}
