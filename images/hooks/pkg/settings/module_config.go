/*
Copyright 2026 Flant JSC

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

package settings

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/module-sdk/pkg"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
)

func HasModuleConfig(ctx context.Context, input *pkg.HookInput) (bool, error) {
	if input == nil || input.DC == nil {
		return false, fmt.Errorf("dependency container is nil")
	}

	k8sClient, err := input.DC.GetK8sClient(addModuleConfigScheme())
	if err != nil {
		return false, fmt.Errorf("get kubernetes client: %w", err)
	}

	var moduleConfig mcapi.ModuleConfig
	err = k8sClient.Get(ctx, client.ObjectKey{Name: ModuleName}, &moduleConfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get ModuleConfig/%s: %w", ModuleName, err)
	}

	if moduleConfig.Spec.Settings == nil {
		return false, nil
	}

	if _, ok := moduleConfig.Spec.Settings["virtualMachineCIDRs"]; !ok {
		return false, nil
	}

	if _, ok := moduleConfig.Spec.Settings["dvcr"]; !ok {
		return false, nil
	}

	return true, nil
}

type moduleConfigSchemeOption struct{}

func (moduleConfigSchemeOption) Apply(optsApplier pkg.KubernetesOptionApplier) {
	optsApplier.WithSchemeBuilder(mcapi.SchemeBuilder)
}

func addModuleConfigScheme() pkg.KubernetesOption {
	return moduleConfigSchemeOption{}
}

func NewModuleConfigForTest(settings map[string]any) *mcapi.ModuleConfig {
	return &mcapi.ModuleConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ModuleConfig",
			APIVersion: "deckhouse.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{Name: ModuleName},
		Spec: mcapi.ModuleConfigSpec{
			Settings: settings,
		},
	}
}
