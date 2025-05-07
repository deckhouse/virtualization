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

package module

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:deepcopy-gen=true
type Module struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Properties ModuleProperties `json:"properties"`
}

// +k8s:deepcopy-gen=true
type ModuleProperties struct {
	Version string `json:"version"`
}

// DeepCopyInto copies the receiver into the given module.
func (in *Module) DeepCopyInto(out *Module) {
	*out = *in
	out.TypeMeta = in.TypeMeta

	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)

	in.Properties.DeepCopyInto(&out.Properties)
}

// DeepCopy creates a deep copy of the module.
func (in *Module) DeepCopy() *Module {
	if in == nil {
		return nil
	}
	out := new(Module)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject implements the runtime.Object interface.
func (in *Module) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := in.DeepCopy()
	return out
}

// DeepCopyInto copies the receiver into the given moduleSpec.
func (in *ModuleProperties) DeepCopyInto(out *ModuleProperties) {
	*out = *in
}

// DeepCopy creates a deep copy of the moduleSpec.
func (in *ModuleProperties) DeepCopy() *ModuleProperties {
	if in == nil {
		return nil
	}
	out := new(ModuleProperties)
	in.DeepCopyInto(out)
	return out
}
