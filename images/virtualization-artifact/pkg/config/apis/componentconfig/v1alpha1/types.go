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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualizationControllerConfiguration struct {
	metav1.TypeMeta `json:",inline"`
	Spec            VirtualizationControllerConfigurationSpec `json:"spec"`
}

type VirtualizationControllerConfigurationSpec struct {
	Namespace                               string                           `json:"namespace"`
	FirmwareImage                           string                           `json:"firmwareImage"`
	VirtControllerName                      string                           `json:"virtControllerName"`
	VirtualMachineCIDRs                     []string                         `json:"virtualMachineCIDRs"`
	VirtualMachineIPLeasesRetentionDuration metav1.Duration                  `json:"virtualMachineIPLeasesRetentionDuration,omitempty"`
	GarbageCollector                        GarbageCollector                 `json:"garbageCollector,omitempty"`
	VirtualImageStorageClassSettings        VirtualImageStorageClassSettings `json:"virtualImageStorageClassSettings,omitempty"`
	VirtualDiskStorageClassSettings         VirtualDiskStorageClassSettings  `json:"virtualDiskStorageClassSettings,omitempty"`
	ImportSettings                          ImportSettings                   `json:"importSettings"`
	DVCR                                    DVCR                             `json:"dvcr"`
	Ingress                                 Ingress                          `json:"ingress"`
}

type GarbageCollector struct {
	VMOP         BaseGCSettings `json:"vmop,omitempty"`
	VMIMigration BaseGCSettings `json:"vmiMigration,omitempty"`
}

type BaseGCSettings struct {
	TTL      metav1.Duration `json:"ttl,omitempty"`
	Schedule string          `json:"schedule,omitempty"`
}

type VirtualImageStorageClassSettings struct {
	AllowedStorageClassNames []string `json:"allowedStorageClassNames,omitempty"`
	DefaultStorageClassName  string   `json:"defaultStorageClassName,omitempty"`
	StorageClassName         string   `json:"storageClassName,omitempty"`
}

type VirtualDiskStorageClassSettings struct {
	AllowedStorageClassNames []string `json:"allowedStorageClassNames,omitempty"`
	DefaultStorageClassName  string   `json:"defaultStorageClassName,omitempty"`
}

type ImportSettings struct {
	ImporterImage string                      `json:"importerImage"`
	UploaderImage string                      `json:"uploaderImage"`
	BounderImage  string                      `json:"bounderImage"`
	Requirements  corev1.ResourceRequirements `json:"requirements,omitempty"`
}

type DVCR struct {
	// AuthSecret is a name of the Secret with docker authentication.
	AuthSecret string `json:"authSecret"`
	// CertsSecret is a name of the TLS Secret with DVCR certificates (only CA cert is used).
	CertsSecret string `json:"certsSecret,omitempty"`
	// RegistryURL is a registry hostname with optional endpoint.
	RegistryURL string `json:"registryURL,omitempty"`
	// InsecureTLS specifies if registry is insecure (trust all certificates). Works for destination only.
	InsecureTLS bool `json:"insecureTLS,omitempty"`
}

type Ingress struct {
	Host      string `json:"host"`
	TLSSecret string `json:"tlsSecret,omitempty"`
	Class     string `json:"class,omitempty"`
}
