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

package config

import (
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/deckhouse/virtualization-controller/pkg/config/apis/componentconfig"
	"github.com/deckhouse/virtualization-controller/pkg/config/apis/componentconfig/install"
	"github.com/deckhouse/virtualization-controller/pkg/config/apis/componentconfig/v1alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
)

func Load(path string) (*componentconfig.VirtualizationControllerConfiguration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decodedConfig, err := decodeVirtualizationControllerConfiguration(data)
	if err != nil {
		return nil, err
	}
	return decodedConfig, nil
}

func decodeVirtualizationControllerConfiguration(configBytes []byte) (*componentconfig.VirtualizationControllerConfiguration, error) {
	scheme := runtime.NewScheme()
	install.Install(scheme)
	codecs := serializer.NewCodecFactory(scheme)

	internalConfig := &componentconfig.VirtualizationControllerConfiguration{}
	decoder := codecs.UniversalDecoder(v1alpha1.SchemeGroupVersion, componentconfig.SchemeGroupVersion)
	if err := runtime.DecodeInto(decoder, configBytes, internalConfig); err != nil {
		return nil, fmt.Errorf("failed to decode VirtualizationControllerConfiguration: %w", err)
	}

	setVirtualizationControllerConfigurationSpecDefaults(&internalConfig.Spec)

	err := validateVirtualizationControllerConfigurationSpec(internalConfig.Spec)
	if err != nil {
		return nil, err
	}

	return internalConfig, nil
}

func setVirtualizationControllerConfigurationSpecDefaults(spec *componentconfig.VirtualizationControllerConfigurationSpec) {
	if spec.VirtualMachineIPLeasesRetentionDuration.Duration == 0 {
		spec.VirtualMachineIPLeasesRetentionDuration = metav1.Duration{Duration: 10 * time.Minute}
	}
	if spec.GarbageCollector.VMOP.TTL.Duration == 0 {
		spec.GarbageCollector.VMOP.TTL.Duration = 24 * time.Hour
	}
	if spec.GarbageCollector.VMIMigration.TTL.Duration == 0 {
		spec.GarbageCollector.VMIMigration.TTL.Duration = 24 * time.Hour
	}
	if spec.GarbageCollector.VMOP.Schedule == "" {
		spec.GarbageCollector.VMOP.Schedule = "0 * * * *"
	}
	if spec.GarbageCollector.VMIMigration.Schedule == "" {
		spec.GarbageCollector.VMIMigration.Schedule = "0 * * * *"
	}

	if spec.ImportSettings.Requirements.Limits == nil && spec.ImportSettings.Requirements.Requests == nil {
		spec.ImportSettings.Requirements.Limits = corev1.ResourceList{
			"cpu":    resource.MustParse("750m"),
			"memory": resource.MustParse("600Mi"),
		}
		spec.ImportSettings.Requirements.Requests = corev1.ResourceList{
			"cpu":    resource.MustParse("100m"),
			"memory": resource.MustParse("60Mi"),
		}
	}
}

func validateVirtualizationControllerConfigurationSpec(spec componentconfig.VirtualizationControllerConfigurationSpec) error {
	if spec.Namespace == "" {
		return fmt.Errorf("spec.namespace is required")
	}

	if spec.FirmwareImage == "" {
		return fmt.Errorf("spec.firmwareImage is required")
	}

	if spec.VirtControllerName == "" {
		return fmt.Errorf("spec.virtControllerName is required")
	}

	if len(spec.VirtualMachineCIDRs) == 0 {
		return fmt.Errorf("spec.virtualMachineCIDRs is required")
	}

	if spec.ImportSettings.ImporterImage == "" {
		return fmt.Errorf("spec.importSettings.importerImage is required")
	}

	if spec.ImportSettings.UploaderImage == "" {
		return fmt.Errorf("spec.importSettings.uploaderImage is required")
	}

	if spec.ImportSettings.BounderImage == "" {
		return fmt.Errorf("spec.importSettings.bounderImage is required")
	}

	if spec.DVCR.AuthSecret == "" {
		return fmt.Errorf("spec.dvcr.authSecret is required")
	}

	if spec.Ingress.Host == "" {
		return fmt.Errorf("spec.ingress.host is required")
	}

	return nil
}

func ToLegacyDVCR(config *componentconfig.VirtualizationControllerConfiguration) *dvcr.Settings {
	insecureTLS := "false"
	if config.Spec.DVCR.InsecureTLS {
		insecureTLS = "true"
	}
	return &dvcr.Settings{
		AuthSecret:           config.Spec.DVCR.AuthSecret,
		AuthSecretNamespace:  config.Spec.Namespace,
		CertsSecret:          config.Spec.DVCR.CertsSecret,
		CertsSecretNamespace: config.Spec.Namespace,
		RegistryURL:          config.Spec.DVCR.RegistryURL,
		InsecureTLS:          insecureTLS,
		UploaderIngressSettings: dvcr.UploaderIngressSettings{
			Host:               config.Spec.Ingress.Host,
			TLSSecret:          config.Spec.Ingress.TLSSecret,
			TLSSecretNamespace: config.Spec.Namespace,
			Class:              config.Spec.Ingress.Class,
		},
	}
}

func ToLegacyVirtualImageStorageClassSettings(config *componentconfig.VirtualizationControllerConfiguration) VirtualImageStorageClassSettings {
	return VirtualImageStorageClassSettings{
		AllowedStorageClassNames: config.Spec.VirtualImageStorageClassSettings.AllowedStorageClassNames,
		DefaultStorageClassName:  config.Spec.VirtualImageStorageClassSettings.DefaultStorageClassName,
		StorageClassName:         config.Spec.VirtualImageStorageClassSettings.StorageClassName,
	}
}

func ToLegacyVirtualDiskStorageClassSettings(config *componentconfig.VirtualizationControllerConfiguration) VirtualDiskStorageClassSettings {
	return VirtualDiskStorageClassSettings{
		AllowedStorageClassNames: config.Spec.VirtualDiskStorageClassSettings.AllowedStorageClassNames,
		DefaultStorageClassName:  config.Spec.VirtualDiskStorageClassSettings.DefaultStorageClassName,
	}
}
