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

import "k8s.io/apimachinery/pkg/runtime"

func (in *ModuleConfig) DeepCopyInto(out *ModuleConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

func (in *ModuleConfig) DeepCopy() *ModuleConfig {
	if in == nil {
		return nil
	}
	out := new(ModuleConfig)
	in.DeepCopyInto(out)
	return out
}

func (in *ModuleConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ModuleConfigList) DeepCopyInto(out *ModuleConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ModuleConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *ModuleConfigList) DeepCopy() *ModuleConfigList {
	if in == nil {
		return nil
	}
	out := new(ModuleConfigList)
	in.DeepCopyInto(out)
	return out
}

func (in *ModuleConfigList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *ModuleConfigSpec) DeepCopyInto(out *ModuleConfigSpec) {
	*out = *in
	in.Settings.DeepCopyInto(&out.Settings)
	if in.Enabled != nil {
		in, out := &in.Enabled, &out.Enabled
		*out = new(bool)
		**out = **in
	}
}

func (in *ModuleConfigSpec) DeepCopy() *ModuleConfigSpec {
	if in == nil {
		return nil
	}
	out := new(ModuleConfigSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *ModuleConfigStatus) DeepCopyInto(out *ModuleConfigStatus) {
	*out = *in
}

func (in *ModuleConfigStatus) DeepCopy() *ModuleConfigStatus {
	if in == nil {
		return nil
	}
	out := new(ModuleConfigStatus)
	in.DeepCopyInto(out)
	return out
}

func (v *SettingsValues) DeepCopy() *SettingsValues {
	nmap := make(map[string]interface{}, len(*v))

	for key, value := range *v {
		nmap[key] = value
	}

	vv := SettingsValues(nmap)

	return &vv
}

func (v SettingsValues) DeepCopyInto(out *SettingsValues) {
	{
		v := &v
		clone := v.DeepCopy()
		*out = *clone
		return
	}
}
