//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright Flant JSC

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

// Code generated by conversion-gen. DO NOT EDIT.

package v1alpha2

import (
	url "net/url"
	unsafe "unsafe"

	subresources "github.com/deckhouse/virtualization/api/subresources"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	conversion "k8s.io/apimachinery/pkg/conversion"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func init() {
	localSchemeBuilder.Register(RegisterConversions)
}

// RegisterConversions adds conversion functions to the given scheme.
// Public to allow building arbitrary schemes.
func RegisterConversions(s *runtime.Scheme) error {
	if err := s.AddGeneratedConversionFunc((*VirtualMachineAddVolume)(nil), (*subresources.VirtualMachineAddVolume)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha2_VirtualMachineAddVolume_To_subresources_VirtualMachineAddVolume(a.(*VirtualMachineAddVolume), b.(*subresources.VirtualMachineAddVolume), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*subresources.VirtualMachineAddVolume)(nil), (*VirtualMachineAddVolume)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_subresources_VirtualMachineAddVolume_To_v1alpha2_VirtualMachineAddVolume(a.(*subresources.VirtualMachineAddVolume), b.(*VirtualMachineAddVolume), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*VirtualMachineConsole)(nil), (*subresources.VirtualMachineConsole)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha2_VirtualMachineConsole_To_subresources_VirtualMachineConsole(a.(*VirtualMachineConsole), b.(*subresources.VirtualMachineConsole), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*subresources.VirtualMachineConsole)(nil), (*VirtualMachineConsole)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_subresources_VirtualMachineConsole_To_v1alpha2_VirtualMachineConsole(a.(*subresources.VirtualMachineConsole), b.(*VirtualMachineConsole), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*VirtualMachineFreeze)(nil), (*subresources.VirtualMachineFreeze)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha2_VirtualMachineFreeze_To_subresources_VirtualMachineFreeze(a.(*VirtualMachineFreeze), b.(*subresources.VirtualMachineFreeze), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*subresources.VirtualMachineFreeze)(nil), (*VirtualMachineFreeze)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_subresources_VirtualMachineFreeze_To_v1alpha2_VirtualMachineFreeze(a.(*subresources.VirtualMachineFreeze), b.(*VirtualMachineFreeze), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*VirtualMachineMigrate)(nil), (*subresources.VirtualMachineMigrate)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha2_VirtualMachineMigrate_To_subresources_VirtualMachineMigrate(a.(*VirtualMachineMigrate), b.(*subresources.VirtualMachineMigrate), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*subresources.VirtualMachineMigrate)(nil), (*VirtualMachineMigrate)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_subresources_VirtualMachineMigrate_To_v1alpha2_VirtualMachineMigrate(a.(*subresources.VirtualMachineMigrate), b.(*VirtualMachineMigrate), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*VirtualMachinePortForward)(nil), (*subresources.VirtualMachinePortForward)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha2_VirtualMachinePortForward_To_subresources_VirtualMachinePortForward(a.(*VirtualMachinePortForward), b.(*subresources.VirtualMachinePortForward), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*subresources.VirtualMachinePortForward)(nil), (*VirtualMachinePortForward)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_subresources_VirtualMachinePortForward_To_v1alpha2_VirtualMachinePortForward(a.(*subresources.VirtualMachinePortForward), b.(*VirtualMachinePortForward), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*VirtualMachineRemoveVolume)(nil), (*subresources.VirtualMachineRemoveVolume)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha2_VirtualMachineRemoveVolume_To_subresources_VirtualMachineRemoveVolume(a.(*VirtualMachineRemoveVolume), b.(*subresources.VirtualMachineRemoveVolume), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*subresources.VirtualMachineRemoveVolume)(nil), (*VirtualMachineRemoveVolume)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_subresources_VirtualMachineRemoveVolume_To_v1alpha2_VirtualMachineRemoveVolume(a.(*subresources.VirtualMachineRemoveVolume), b.(*VirtualMachineRemoveVolume), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*VirtualMachineUnfreeze)(nil), (*subresources.VirtualMachineUnfreeze)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha2_VirtualMachineUnfreeze_To_subresources_VirtualMachineUnfreeze(a.(*VirtualMachineUnfreeze), b.(*subresources.VirtualMachineUnfreeze), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*subresources.VirtualMachineUnfreeze)(nil), (*VirtualMachineUnfreeze)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_subresources_VirtualMachineUnfreeze_To_v1alpha2_VirtualMachineUnfreeze(a.(*subresources.VirtualMachineUnfreeze), b.(*VirtualMachineUnfreeze), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*VirtualMachineVNC)(nil), (*subresources.VirtualMachineVNC)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1alpha2_VirtualMachineVNC_To_subresources_VirtualMachineVNC(a.(*VirtualMachineVNC), b.(*subresources.VirtualMachineVNC), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*subresources.VirtualMachineVNC)(nil), (*VirtualMachineVNC)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_subresources_VirtualMachineVNC_To_v1alpha2_VirtualMachineVNC(a.(*subresources.VirtualMachineVNC), b.(*VirtualMachineVNC), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*url.Values)(nil), (*VirtualMachineAddVolume)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_url_Values_To_v1alpha2_VirtualMachineAddVolume(a.(*url.Values), b.(*VirtualMachineAddVolume), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*url.Values)(nil), (*VirtualMachineConsole)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_url_Values_To_v1alpha2_VirtualMachineConsole(a.(*url.Values), b.(*VirtualMachineConsole), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*url.Values)(nil), (*VirtualMachineFreeze)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_url_Values_To_v1alpha2_VirtualMachineFreeze(a.(*url.Values), b.(*VirtualMachineFreeze), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*url.Values)(nil), (*VirtualMachineMigrate)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_url_Values_To_v1alpha2_VirtualMachineMigrate(a.(*url.Values), b.(*VirtualMachineMigrate), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*url.Values)(nil), (*VirtualMachinePortForward)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_url_Values_To_v1alpha2_VirtualMachinePortForward(a.(*url.Values), b.(*VirtualMachinePortForward), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*url.Values)(nil), (*VirtualMachineRemoveVolume)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_url_Values_To_v1alpha2_VirtualMachineRemoveVolume(a.(*url.Values), b.(*VirtualMachineRemoveVolume), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*url.Values)(nil), (*VirtualMachineUnfreeze)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_url_Values_To_v1alpha2_VirtualMachineUnfreeze(a.(*url.Values), b.(*VirtualMachineUnfreeze), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*url.Values)(nil), (*VirtualMachineVNC)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_url_Values_To_v1alpha2_VirtualMachineVNC(a.(*url.Values), b.(*VirtualMachineVNC), scope)
	}); err != nil {
		return err
	}
	return nil
}

func autoConvert_v1alpha2_VirtualMachineAddVolume_To_subresources_VirtualMachineAddVolume(in *VirtualMachineAddVolume, out *subresources.VirtualMachineAddVolume, s conversion.Scope) error {
	return nil
}

// Convert_v1alpha2_VirtualMachineAddVolume_To_subresources_VirtualMachineAddVolume is an autogenerated conversion function.
func Convert_v1alpha2_VirtualMachineAddVolume_To_subresources_VirtualMachineAddVolume(in *VirtualMachineAddVolume, out *subresources.VirtualMachineAddVolume, s conversion.Scope) error {
	return autoConvert_v1alpha2_VirtualMachineAddVolume_To_subresources_VirtualMachineAddVolume(in, out, s)
}

func autoConvert_subresources_VirtualMachineAddVolume_To_v1alpha2_VirtualMachineAddVolume(in *subresources.VirtualMachineAddVolume, out *VirtualMachineAddVolume, s conversion.Scope) error {
	return nil
}

// Convert_subresources_VirtualMachineAddVolume_To_v1alpha2_VirtualMachineAddVolume is an autogenerated conversion function.
func Convert_subresources_VirtualMachineAddVolume_To_v1alpha2_VirtualMachineAddVolume(in *subresources.VirtualMachineAddVolume, out *VirtualMachineAddVolume, s conversion.Scope) error {
	return autoConvert_subresources_VirtualMachineAddVolume_To_v1alpha2_VirtualMachineAddVolume(in, out, s)
}

func autoConvert_url_Values_To_v1alpha2_VirtualMachineAddVolume(in *url.Values, out *VirtualMachineAddVolume, s conversion.Scope) error {
	// WARNING: Field TypeMeta does not have json tag, skipping.

	return nil
}

// Convert_url_Values_To_v1alpha2_VirtualMachineAddVolume is an autogenerated conversion function.
func Convert_url_Values_To_v1alpha2_VirtualMachineAddVolume(in *url.Values, out *VirtualMachineAddVolume, s conversion.Scope) error {
	return autoConvert_url_Values_To_v1alpha2_VirtualMachineAddVolume(in, out, s)
}

func autoConvert_v1alpha2_VirtualMachineConsole_To_subresources_VirtualMachineConsole(in *VirtualMachineConsole, out *subresources.VirtualMachineConsole, s conversion.Scope) error {
	return nil
}

// Convert_v1alpha2_VirtualMachineConsole_To_subresources_VirtualMachineConsole is an autogenerated conversion function.
func Convert_v1alpha2_VirtualMachineConsole_To_subresources_VirtualMachineConsole(in *VirtualMachineConsole, out *subresources.VirtualMachineConsole, s conversion.Scope) error {
	return autoConvert_v1alpha2_VirtualMachineConsole_To_subresources_VirtualMachineConsole(in, out, s)
}

func autoConvert_subresources_VirtualMachineConsole_To_v1alpha2_VirtualMachineConsole(in *subresources.VirtualMachineConsole, out *VirtualMachineConsole, s conversion.Scope) error {
	return nil
}

// Convert_subresources_VirtualMachineConsole_To_v1alpha2_VirtualMachineConsole is an autogenerated conversion function.
func Convert_subresources_VirtualMachineConsole_To_v1alpha2_VirtualMachineConsole(in *subresources.VirtualMachineConsole, out *VirtualMachineConsole, s conversion.Scope) error {
	return autoConvert_subresources_VirtualMachineConsole_To_v1alpha2_VirtualMachineConsole(in, out, s)
}

func autoConvert_url_Values_To_v1alpha2_VirtualMachineConsole(in *url.Values, out *VirtualMachineConsole, s conversion.Scope) error {
	// WARNING: Field TypeMeta does not have json tag, skipping.

	return nil
}

// Convert_url_Values_To_v1alpha2_VirtualMachineConsole is an autogenerated conversion function.
func Convert_url_Values_To_v1alpha2_VirtualMachineConsole(in *url.Values, out *VirtualMachineConsole, s conversion.Scope) error {
	return autoConvert_url_Values_To_v1alpha2_VirtualMachineConsole(in, out, s)
}

func autoConvert_v1alpha2_VirtualMachineFreeze_To_subresources_VirtualMachineFreeze(in *VirtualMachineFreeze, out *subresources.VirtualMachineFreeze, s conversion.Scope) error {
	out.UnfreezeTimeout = (*v1.Duration)(unsafe.Pointer(in.UnfreezeTimeout))
	return nil
}

// Convert_v1alpha2_VirtualMachineFreeze_To_subresources_VirtualMachineFreeze is an autogenerated conversion function.
func Convert_v1alpha2_VirtualMachineFreeze_To_subresources_VirtualMachineFreeze(in *VirtualMachineFreeze, out *subresources.VirtualMachineFreeze, s conversion.Scope) error {
	return autoConvert_v1alpha2_VirtualMachineFreeze_To_subresources_VirtualMachineFreeze(in, out, s)
}

func autoConvert_subresources_VirtualMachineFreeze_To_v1alpha2_VirtualMachineFreeze(in *subresources.VirtualMachineFreeze, out *VirtualMachineFreeze, s conversion.Scope) error {
	out.UnfreezeTimeout = (*v1.Duration)(unsafe.Pointer(in.UnfreezeTimeout))
	return nil
}

// Convert_subresources_VirtualMachineFreeze_To_v1alpha2_VirtualMachineFreeze is an autogenerated conversion function.
func Convert_subresources_VirtualMachineFreeze_To_v1alpha2_VirtualMachineFreeze(in *subresources.VirtualMachineFreeze, out *VirtualMachineFreeze, s conversion.Scope) error {
	return autoConvert_subresources_VirtualMachineFreeze_To_v1alpha2_VirtualMachineFreeze(in, out, s)
}

func autoConvert_url_Values_To_v1alpha2_VirtualMachineFreeze(in *url.Values, out *VirtualMachineFreeze, s conversion.Scope) error {
	// WARNING: Field TypeMeta does not have json tag, skipping.

	if values, ok := map[string][]string(*in)["unfreezeTimeout"]; ok && len(values) > 0 {
		// FIXME: out.UnfreezeTimeout is of not yet supported type and requires manual conversion
	} else {
		out.UnfreezeTimeout = nil
	}
	return nil
}

// Convert_url_Values_To_v1alpha2_VirtualMachineFreeze is an autogenerated conversion function.
func Convert_url_Values_To_v1alpha2_VirtualMachineFreeze(in *url.Values, out *VirtualMachineFreeze, s conversion.Scope) error {
	return autoConvert_url_Values_To_v1alpha2_VirtualMachineFreeze(in, out, s)
}

func autoConvert_v1alpha2_VirtualMachineMigrate_To_subresources_VirtualMachineMigrate(in *VirtualMachineMigrate, out *subresources.VirtualMachineMigrate, s conversion.Scope) error {
	out.DryRun = *(*[]string)(unsafe.Pointer(&in.DryRun))
	return nil
}

// Convert_v1alpha2_VirtualMachineMigrate_To_subresources_VirtualMachineMigrate is an autogenerated conversion function.
func Convert_v1alpha2_VirtualMachineMigrate_To_subresources_VirtualMachineMigrate(in *VirtualMachineMigrate, out *subresources.VirtualMachineMigrate, s conversion.Scope) error {
	return autoConvert_v1alpha2_VirtualMachineMigrate_To_subresources_VirtualMachineMigrate(in, out, s)
}

func autoConvert_subresources_VirtualMachineMigrate_To_v1alpha2_VirtualMachineMigrate(in *subresources.VirtualMachineMigrate, out *VirtualMachineMigrate, s conversion.Scope) error {
	out.DryRun = *(*[]string)(unsafe.Pointer(&in.DryRun))
	return nil
}

// Convert_subresources_VirtualMachineMigrate_To_v1alpha2_VirtualMachineMigrate is an autogenerated conversion function.
func Convert_subresources_VirtualMachineMigrate_To_v1alpha2_VirtualMachineMigrate(in *subresources.VirtualMachineMigrate, out *VirtualMachineMigrate, s conversion.Scope) error {
	return autoConvert_subresources_VirtualMachineMigrate_To_v1alpha2_VirtualMachineMigrate(in, out, s)
}

func autoConvert_url_Values_To_v1alpha2_VirtualMachineMigrate(in *url.Values, out *VirtualMachineMigrate, s conversion.Scope) error {
	// WARNING: Field TypeMeta does not have json tag, skipping.

	if values, ok := map[string][]string(*in)["dryRun"]; ok && len(values) > 0 {
		out.DryRun = *(*[]string)(unsafe.Pointer(&values))
	} else {
		out.DryRun = nil
	}
	return nil
}

// Convert_url_Values_To_v1alpha2_VirtualMachineMigrate is an autogenerated conversion function.
func Convert_url_Values_To_v1alpha2_VirtualMachineMigrate(in *url.Values, out *VirtualMachineMigrate, s conversion.Scope) error {
	return autoConvert_url_Values_To_v1alpha2_VirtualMachineMigrate(in, out, s)
}

func autoConvert_v1alpha2_VirtualMachinePortForward_To_subresources_VirtualMachinePortForward(in *VirtualMachinePortForward, out *subresources.VirtualMachinePortForward, s conversion.Scope) error {
	out.Protocol = in.Protocol
	out.Port = in.Port
	return nil
}

// Convert_v1alpha2_VirtualMachinePortForward_To_subresources_VirtualMachinePortForward is an autogenerated conversion function.
func Convert_v1alpha2_VirtualMachinePortForward_To_subresources_VirtualMachinePortForward(in *VirtualMachinePortForward, out *subresources.VirtualMachinePortForward, s conversion.Scope) error {
	return autoConvert_v1alpha2_VirtualMachinePortForward_To_subresources_VirtualMachinePortForward(in, out, s)
}

func autoConvert_subresources_VirtualMachinePortForward_To_v1alpha2_VirtualMachinePortForward(in *subresources.VirtualMachinePortForward, out *VirtualMachinePortForward, s conversion.Scope) error {
	out.Protocol = in.Protocol
	out.Port = in.Port
	return nil
}

// Convert_subresources_VirtualMachinePortForward_To_v1alpha2_VirtualMachinePortForward is an autogenerated conversion function.
func Convert_subresources_VirtualMachinePortForward_To_v1alpha2_VirtualMachinePortForward(in *subresources.VirtualMachinePortForward, out *VirtualMachinePortForward, s conversion.Scope) error {
	return autoConvert_subresources_VirtualMachinePortForward_To_v1alpha2_VirtualMachinePortForward(in, out, s)
}

func autoConvert_url_Values_To_v1alpha2_VirtualMachinePortForward(in *url.Values, out *VirtualMachinePortForward, s conversion.Scope) error {
	// WARNING: Field TypeMeta does not have json tag, skipping.

	if values, ok := map[string][]string(*in)["protocol"]; ok && len(values) > 0 {
		if err := runtime.Convert_Slice_string_To_string(&values, &out.Protocol, s); err != nil {
			return err
		}
	} else {
		out.Protocol = ""
	}
	if values, ok := map[string][]string(*in)["port"]; ok && len(values) > 0 {
		if err := runtime.Convert_Slice_string_To_int(&values, &out.Port, s); err != nil {
			return err
		}
	} else {
		out.Port = 0
	}
	return nil
}

// Convert_url_Values_To_v1alpha2_VirtualMachinePortForward is an autogenerated conversion function.
func Convert_url_Values_To_v1alpha2_VirtualMachinePortForward(in *url.Values, out *VirtualMachinePortForward, s conversion.Scope) error {
	return autoConvert_url_Values_To_v1alpha2_VirtualMachinePortForward(in, out, s)
}

func autoConvert_v1alpha2_VirtualMachineRemoveVolume_To_subresources_VirtualMachineRemoveVolume(in *VirtualMachineRemoveVolume, out *subresources.VirtualMachineRemoveVolume, s conversion.Scope) error {
	return nil
}

// Convert_v1alpha2_VirtualMachineRemoveVolume_To_subresources_VirtualMachineRemoveVolume is an autogenerated conversion function.
func Convert_v1alpha2_VirtualMachineRemoveVolume_To_subresources_VirtualMachineRemoveVolume(in *VirtualMachineRemoveVolume, out *subresources.VirtualMachineRemoveVolume, s conversion.Scope) error {
	return autoConvert_v1alpha2_VirtualMachineRemoveVolume_To_subresources_VirtualMachineRemoveVolume(in, out, s)
}

func autoConvert_subresources_VirtualMachineRemoveVolume_To_v1alpha2_VirtualMachineRemoveVolume(in *subresources.VirtualMachineRemoveVolume, out *VirtualMachineRemoveVolume, s conversion.Scope) error {
	return nil
}

// Convert_subresources_VirtualMachineRemoveVolume_To_v1alpha2_VirtualMachineRemoveVolume is an autogenerated conversion function.
func Convert_subresources_VirtualMachineRemoveVolume_To_v1alpha2_VirtualMachineRemoveVolume(in *subresources.VirtualMachineRemoveVolume, out *VirtualMachineRemoveVolume, s conversion.Scope) error {
	return autoConvert_subresources_VirtualMachineRemoveVolume_To_v1alpha2_VirtualMachineRemoveVolume(in, out, s)
}

func autoConvert_url_Values_To_v1alpha2_VirtualMachineRemoveVolume(in *url.Values, out *VirtualMachineRemoveVolume, s conversion.Scope) error {
	// WARNING: Field TypeMeta does not have json tag, skipping.

	return nil
}

// Convert_url_Values_To_v1alpha2_VirtualMachineRemoveVolume is an autogenerated conversion function.
func Convert_url_Values_To_v1alpha2_VirtualMachineRemoveVolume(in *url.Values, out *VirtualMachineRemoveVolume, s conversion.Scope) error {
	return autoConvert_url_Values_To_v1alpha2_VirtualMachineRemoveVolume(in, out, s)
}

func autoConvert_v1alpha2_VirtualMachineUnfreeze_To_subresources_VirtualMachineUnfreeze(in *VirtualMachineUnfreeze, out *subresources.VirtualMachineUnfreeze, s conversion.Scope) error {
	return nil
}

// Convert_v1alpha2_VirtualMachineUnfreeze_To_subresources_VirtualMachineUnfreeze is an autogenerated conversion function.
func Convert_v1alpha2_VirtualMachineUnfreeze_To_subresources_VirtualMachineUnfreeze(in *VirtualMachineUnfreeze, out *subresources.VirtualMachineUnfreeze, s conversion.Scope) error {
	return autoConvert_v1alpha2_VirtualMachineUnfreeze_To_subresources_VirtualMachineUnfreeze(in, out, s)
}

func autoConvert_subresources_VirtualMachineUnfreeze_To_v1alpha2_VirtualMachineUnfreeze(in *subresources.VirtualMachineUnfreeze, out *VirtualMachineUnfreeze, s conversion.Scope) error {
	return nil
}

// Convert_subresources_VirtualMachineUnfreeze_To_v1alpha2_VirtualMachineUnfreeze is an autogenerated conversion function.
func Convert_subresources_VirtualMachineUnfreeze_To_v1alpha2_VirtualMachineUnfreeze(in *subresources.VirtualMachineUnfreeze, out *VirtualMachineUnfreeze, s conversion.Scope) error {
	return autoConvert_subresources_VirtualMachineUnfreeze_To_v1alpha2_VirtualMachineUnfreeze(in, out, s)
}

func autoConvert_url_Values_To_v1alpha2_VirtualMachineUnfreeze(in *url.Values, out *VirtualMachineUnfreeze, s conversion.Scope) error {
	// WARNING: Field TypeMeta does not have json tag, skipping.

	return nil
}

// Convert_url_Values_To_v1alpha2_VirtualMachineUnfreeze is an autogenerated conversion function.
func Convert_url_Values_To_v1alpha2_VirtualMachineUnfreeze(in *url.Values, out *VirtualMachineUnfreeze, s conversion.Scope) error {
	return autoConvert_url_Values_To_v1alpha2_VirtualMachineUnfreeze(in, out, s)
}

func autoConvert_v1alpha2_VirtualMachineVNC_To_subresources_VirtualMachineVNC(in *VirtualMachineVNC, out *subresources.VirtualMachineVNC, s conversion.Scope) error {
	return nil
}

// Convert_v1alpha2_VirtualMachineVNC_To_subresources_VirtualMachineVNC is an autogenerated conversion function.
func Convert_v1alpha2_VirtualMachineVNC_To_subresources_VirtualMachineVNC(in *VirtualMachineVNC, out *subresources.VirtualMachineVNC, s conversion.Scope) error {
	return autoConvert_v1alpha2_VirtualMachineVNC_To_subresources_VirtualMachineVNC(in, out, s)
}

func autoConvert_subresources_VirtualMachineVNC_To_v1alpha2_VirtualMachineVNC(in *subresources.VirtualMachineVNC, out *VirtualMachineVNC, s conversion.Scope) error {
	return nil
}

// Convert_subresources_VirtualMachineVNC_To_v1alpha2_VirtualMachineVNC is an autogenerated conversion function.
func Convert_subresources_VirtualMachineVNC_To_v1alpha2_VirtualMachineVNC(in *subresources.VirtualMachineVNC, out *VirtualMachineVNC, s conversion.Scope) error {
	return autoConvert_subresources_VirtualMachineVNC_To_v1alpha2_VirtualMachineVNC(in, out, s)
}

func autoConvert_url_Values_To_v1alpha2_VirtualMachineVNC(in *url.Values, out *VirtualMachineVNC, s conversion.Scope) error {
	// WARNING: Field TypeMeta does not have json tag, skipping.

	return nil
}

// Convert_url_Values_To_v1alpha2_VirtualMachineVNC is an autogenerated conversion function.
func Convert_url_Values_To_v1alpha2_VirtualMachineVNC(in *url.Values, out *VirtualMachineVNC, s conversion.Scope) error {
	return autoConvert_url_Values_To_v1alpha2_VirtualMachineVNC(in, out, s)
}
