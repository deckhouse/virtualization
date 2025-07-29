package service

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineClassService struct {
	client client.Client
}

func NewVirtualMachineClassService(client client.Client) *VirtualMachineClassService {
	return &VirtualMachineClassService{
		client: client,
	}
}

func (v *VirtualMachineClassService) IsDefault(vmClass *v1alpha2.VirtualMachineClass) bool {
	if vmClass == nil {
		return false
	}
	classAnnotations := vmClass.GetAnnotations()
	if classAnnotations == nil {
		return false
	}
	_, ok := classAnnotations[annotations.AnnVirtualMachineClassDefault]
	return ok
}

func (v *VirtualMachineClassService) GetDefault(classes *v1alpha2.VirtualMachineClassList) (*v1alpha2.VirtualMachineClass, error) {
	if classes == nil {
		return nil, nil
	}

	var defaultClass *v1alpha2.VirtualMachineClass
	for i := range classes.Items {
		if !v.IsDefault(&classes.Items[i]) {
			continue
		}
		if defaultClass != nil {
			return nil, fmt.Errorf("multiple default classes are found (%s, %s)", defaultClass.GetName(), classes.Items[i].GetName())
		}
		defaultClass = &classes.Items[i]
	}
	return defaultClass, nil
}
