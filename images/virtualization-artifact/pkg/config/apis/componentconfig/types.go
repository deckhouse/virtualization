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

package componentconfig

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualizationControllerConfiguration struct {
	metav1.TypeMeta `json:",inline"`
	Spec            VirtualizationControllerConfigurationSpec
}

type VirtualizationControllerConfigurationSpec struct {
	Namespace                               string
	FirmwareImage                           string
	VirtControllerName                      string
	VirtualMachineCIDRs                     []string
	VirtualMachineIPLeasesRetentionDuration metav1.Duration
	GarbageCollector                        GarbageCollector
	VirtualImageStorageClassSettings        VirtualImageStorageClassSettings
	VirtualDiskStorageClassSettings         VirtualDiskStorageClassSettings
	ImportSettings                          ImportSettings
	DVCR                                    DVCR
	Ingress                                 Ingress
}

type GarbageCollector struct {
	VMOP         BaseGCSettings
	VMIMigration BaseGCSettings
}

type BaseGCSettings struct {
	TTL      metav1.Duration
	Schedule string
}

type VirtualImageStorageClassSettings struct {
	AllowedStorageClassNames []string
	DefaultStorageClassName  string
	StorageClassName         string
}

type VirtualDiskStorageClassSettings struct {
	AllowedStorageClassNames []string
	DefaultStorageClassName  string
}

type ImportSettings struct {
	ImporterImage string
	UploaderImage string
	BounderImage  string
	Requirements  corev1.ResourceRequirements
}

type DVCR struct {
	// AuthSecret is a name of the Secret with docker authentication.
	AuthSecret string
	// CertsSecret is a name of the TLS Secret with DVCR certificates (only CA cert is used).
	CertsSecret string
	// RegistryURL is a registry hostname with optional endpoint.
	RegistryURL string
	// InsecureTLS specifies if registry is insecure (trust all certificates). Works for destination only.
	InsecureTLS bool
}

type Ingress struct {
	Host      string
	TLSSecret string
	Class     string
}
