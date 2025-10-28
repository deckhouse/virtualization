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

package framework

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dv1alpha1 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha1"
)

func (f *Framework) GetModuleConfig(name string) (*dv1alpha1.ModuleConfig, error) {
	mc := &dv1alpha1.ModuleConfig{}
	err := f.GenericClient().Get(context.Background(), client.ObjectKey{Name: name}, mc)
	return mc, err
}

func (f *Framework) GetVirtualizationModuleConfig() (*VirtualizationModuleConfig, error) {
	mc, err := f.GetModuleConfig("virtualization")
	if err != nil {
		return nil, err
	}
	return convertToVirtualizationModuleConfig(mc)
}

func convertToVirtualizationModuleConfig(mc *dv1alpha1.ModuleConfig) (*VirtualizationModuleConfig, error) {
	bytes, err := json.Marshal(mc)
	if err != nil {
		return nil, err
	}
	var vc VirtualizationModuleConfig
	err = json.Unmarshal(bytes, &vc)
	if err != nil {
		return nil, err
	}
	return &vc, nil
}

type VirtualizationModuleConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec VirtualizationModuleConfigSpec `json:"spec"`
}

type VirtualizationModuleConfigSpec struct {
	Enabled  bool                               `json:"enabled"`
	Settings VirtualizationModuleConfigSettings `json:"settings"`
	Version  int                                `json:"version"`
}

type VirtualizationModuleConfigSettings struct {
	Loglevel            string   `json:"logLevel,omitempty"`
	VirtualMachineCIDRs []string `json:"virtualMachineCIDRs"`
	Dvcr                Dvcr     `json:"dvcr"`
	HighAvailability    bool     `json:"highAvailability,omitempty"`
}

type Dvcr struct {
	Storage Storage `json:"storage"`
}

type Storage struct {
	PersistentVolumeClaim map[string]string `json:"persistentVolumeClaim"`
	Type                  string            `json:"type"`
}
