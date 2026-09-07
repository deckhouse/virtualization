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

	"k8s.io/component-base/featuregate"
	"kubevirt.io/api/core"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/validate"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type MetaValidator struct {
	client      client.Client
	featureGate featuregate.FeatureGate
}

func NewMetaValidator(client client.Client, featureGate featuregate.FeatureGate) *MetaValidator {
	return &MetaValidator{client: client, featureGate: featureGate}
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

	if err := v.validateVIOMMUAnnotation(nil, vm); err != nil {
		return nil, err
	}

	return nil, nil
}

// validateVIOMMUAnnotation rejects turning on the emulated IOMMU annotation while
// the VIOMMU feature gate is disabled. A VM that already carries the annotation is
// left updatable, so disabling the gate does not brick existing resources.
func (v *MetaValidator) validateVIOMMUAnnotation(oldVM, newVM *v1alpha2.VirtualMachine) error {
	if newVM.Annotations[annotations.AnnEnableVIOMMU] != "true" {
		return nil
	}
	if oldVM != nil && oldVM.Annotations[annotations.AnnEnableVIOMMU] == "true" {
		return nil
	}
	if v.featureGate.Enabled(featuregates.VIOMMU) {
		return nil
	}
	return fmt.Errorf("the %s annotation requires the VIOMMU feature gate to be enabled in the virtualization module settings", annotations.AnnEnableVIOMMU)
}

func (v *MetaValidator) ValidateUpdate(_ context.Context, oldVM, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
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

	if err := v.validateVIOMMUAnnotation(oldVM, newVM); err != nil {
		return nil, err
	}

	return nil, nil
}
