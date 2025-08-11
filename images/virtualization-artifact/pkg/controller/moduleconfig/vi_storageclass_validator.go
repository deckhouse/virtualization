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

package moduleconfig

import (
	"context"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
)

type viStorageClassValidator struct {
	client client.Client
}

func newViStorageClassValidator(client client.Client) *viStorageClassValidator {
	return &viStorageClassValidator{
		client: client,
	}
}

func (v viStorageClassValidator) ValidateUpdate(ctx context.Context, _, newMC *mcapi.ModuleConfig) (admission.Warnings, error) {
	warnings := make([]string, 0)

	viScSettings := parseViStorageClass(newMC.Spec.Settings)
	if viScSettings.DefaultStorageClassName != "" {
		scWarnings, err := v.validateStorageClass(ctx, viScSettings.DefaultStorageClassName)
		if err != nil {
			return warnings, err
		}
		if len(scWarnings) != 0 {
			warnings = append(warnings, scWarnings...)
		}
	}

	if len(viScSettings.AllowedStorageClassSelector.MatchNames) != 0 {
		for _, sc := range viScSettings.AllowedStorageClassSelector.MatchNames {
			scWarnings, err := v.validateStorageClass(ctx, sc)
			if err != nil {
				return warnings, err
			}
			if len(scWarnings) != 0 {
				warnings = append(warnings, scWarnings...)
			}
		}
	}

	return admission.Warnings{}, nil
}

func (v viStorageClassValidator) validateStorageClass(ctx context.Context, scName string) (admission.Warnings, error) {
	scProfile := &cdiv1.StorageProfile{}
	err := v.client.Get(ctx, client.ObjectKey{Name: scName}, scProfile, &client.GetOptions{})
	if err != nil {
		return admission.Warnings{}, fmt.Errorf("failed to fetch the storage profile %s: %w", scName, err)
	}
	if len(scProfile.Status.ClaimPropertySets) == 0 {
		return admission.Warnings{}, fmt.Errorf("failed to validate claim property sets of the storage profile`: %s", scName)
	}

	for _, cps := range scProfile.Status.ClaimPropertySets {
		if slices.Contains(cps.AccessModes, corev1.ReadWriteMany) && *cps.VolumeMode == corev1.PersistentVolumeBlock {
			return admission.Warnings{}, nil
		}
	}

	return admission.Warnings{}, fmt.Errorf(
		"the storage class %q lacks of capabilities to support 'Virtual Images on PVC' function; use StorageClass that supports volume mode 'Block' and access mode 'ReadWriteMany'",
		scName,
	)
}

type viStorageClassSettings struct {
	DefaultStorageClassName     string
	AllowedStorageClassSelector AllowedStorageClassSelector
}

type AllowedStorageClassSelector struct {
	MatchNames []string
}

func parseViStorageClass(settings mcapi.SettingsValues) *viStorageClassSettings {
	viScSettings := &viStorageClassSettings{}
	if virtualImages, ok := settings["virtualImages"].(map[string]interface{}); ok {
		if defaultClass, ok := virtualImages["defaultStorageClassName"].(string); ok {
			viScSettings.DefaultStorageClassName = defaultClass
		}

		if allowedSelector, ok := virtualImages["allowedStorageClassSelector"].(map[string]interface{}); ok {
			if matchNames, ok := allowedSelector["matchNames"].([]interface{}); ok {
				var matchNameStrings []string
				for _, name := range matchNames {
					if strName, ok := name.(string); ok {
						matchNameStrings = append(matchNameStrings, strName)
					}
				}
				viScSettings.AllowedStorageClassSelector.MatchNames = matchNameStrings
			}
		}
	}

	return viScSettings
}
