/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	return vmClass.Annotations[annotations.AnnVirtualMachineClassDefault] == "true"
}

func (v *VirtualMachineClassService) ValidateDefaultAnnotation(vmClass *v1alpha2.VirtualMachineClass) error {
	if vmClass == nil {
		return nil
	}
	annoValue, ok := vmClass.Annotations[annotations.AnnVirtualMachineClassDefault]
	if ok && annoValue != "true" {
		// Message from validating webhook will be like this:
		// Error from server (Forbidden): admission webhook "vmclass.virtualization-controller.validate.d8-virtualization" denied the request:
		// only 'true' value allowed for annotation that specifies a default class (virtualmachineclass.virtualization.deckhouse.io/is-default-class)
		return fmt.Errorf("only 'true' value allowed for annotation that specifies a default class (%s)", annotations.AnnVirtualMachineClassDefault)
	}
	return nil
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
