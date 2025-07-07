/*
Copyright 2024 Flant JSC

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

package validators

import (
	"context"
	"fmt"
	"strings"

	"kubevirt.io/api/core"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/validate"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type MetaValidator struct {
	client client.Client
}

func NewMetaValidator(client client.Client) *MetaValidator {
	return &MetaValidator{client: client}
}

func (v *MetaValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	if len(vm.Name) > validate.MaxVirtualMachineNameLen {
		return nil, fmt.Errorf("the VirtualMachine name %q is too long: it must be no more than %d characters", vm.Name, validate.MaxVirtualMachineNameLen)
	}

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

func (v *MetaValidator) ValidateUpdate(_ context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
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
