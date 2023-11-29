package ipam

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
)

func New() *IPAM {
	return &IPAM{}
}

type IPAM struct{}

func (m IPAM) IsBound(vmName string, claim *virtv2.VirtualMachineIPAddressClaim) bool {
	if claim == nil {
		return false
	}

	if claim.Status.Phase != virtv2.VirtualMachineIPAddressClaimPhaseBound {
		return false
	}

	anno := claim.GetAnnotations()
	if anno == nil {
		return false
	}

	return anno[common.AnnBoundVirtualMachineName] == vmName
}

func (m IPAM) CheckClaimAvailableForBinding(vmName string, claim *virtv2.VirtualMachineIPAddressClaim) error {
	if claim == nil {
		return errors.New("cannot to bind with empty claim")
	}

	boundVMName := claim.Annotations[common.AnnBoundVirtualMachineName]
	if boundVMName == "" || boundVMName == vmName {
		return nil
	}

	return fmt.Errorf(
		"unable to bind the claim (%s) to the virtual machine (%s): the claim has already been linked to another one",
		claim.Name,
		vmName,
	)
}

func (m IPAM) CreateIPAddressClaim(ctx context.Context, vm *virtv2.VirtualMachine, client client.Client) error {
	return client.Create(ctx, &virtv2.VirtualMachineIPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				common.AnnBoundVirtualMachineName: vm.Name,
			},
			Name:      vm.Name,
			Namespace: vm.Namespace,
		},
		Spec: virtv2.VirtualMachineIPAddressClaimSpec{
			ReclaimPolicy: virtv2.VirtualMachineIPAddressReclaimPolicyDelete,
		},
	})
}

func (m IPAM) BindIPAddressClaim(ctx context.Context, vmName string, claim *virtv2.VirtualMachineIPAddressClaim, client client.Client) error {
	anno := claim.GetAnnotations()
	if anno == nil {
		anno = map[string]string{}
	}

	boundVMName, ok := anno[common.AnnBoundVirtualMachineName]
	if ok {
		// Already in progress.
		if boundVMName == vmName {
			return nil
		}

		return fmt.Errorf("ip address claim %s already bound to another vm", claim.Name)
	}

	anno[common.AnnBoundVirtualMachineName] = vmName

	claim.SetAnnotations(anno)

	return client.Update(ctx, claim)
}

func (m IPAM) DeleteIPAddressClaim(ctx context.Context, claim *virtv2.VirtualMachineIPAddressClaim, client client.Client) error {
	return client.Delete(ctx, claim)
}
