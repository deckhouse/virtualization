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

package v1alpha2

import (
	"fmt"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/deckhouse/virtualization/api/core/v1alpha3"
)

var _ conversion.Convertible = &VirtualMachineClass{}

func (src *VirtualMachineClass) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.VirtualMachineClass)

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = convertSpecV2ToV3(src.Spec)
	dst.Status = convertStatusV2ToV3(src.Status)

	return nil
}

func (dst *VirtualMachineClass) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.VirtualMachineClass)

	dst.ObjectMeta = src.ObjectMeta
	convertedSpec, err := convertSpecV3ToV2(src.Spec)
	if err != nil {
		return err
	}
	dst.Spec = convertedSpec
	dst.Status = convertStatusV3ToV2(src.Status)

	return nil
}

func convertSpecV2ToV3(v2Spec VirtualMachineClassSpec) v1alpha3.VirtualMachineClassSpec {
	v3Spec := v1alpha3.VirtualMachineClassSpec{
		NodeSelector: v1alpha3.NodeSelector{
			MatchLabels:      v2Spec.NodeSelector.MatchLabels,
			MatchExpressions: v2Spec.NodeSelector.MatchExpressions,
		},
		Tolerations: v2Spec.Tolerations,
		CPU: v1alpha3.CPU{
			Type:     v1alpha3.CPUType(v2Spec.CPU.Type),
			Model:    v2Spec.CPU.Model,
			Features: v2Spec.CPU.Features,
			Discovery: v1alpha3.CpuDiscovery{
				NodeSelector: v2Spec.CPU.Discovery.NodeSelector,
			},
		},
	}

	if len(v2Spec.SizingPolicies) > 0 {
		v3Spec.SizingPolicies = make([]v1alpha3.SizingPolicy, len(v2Spec.SizingPolicies))
		for i, v2Policy := range v2Spec.SizingPolicies {
			v3Policy := v1alpha3.SizingPolicy{
				DedicatedCores: v2Policy.DedicatedCores,
			}

			if v2Policy.Memory != nil {
				v3Policy.Memory = &v1alpha3.SizingPolicyMemory{
					MemoryMinMax: v1alpha3.MemoryMinMax{
						Min: v2Policy.Memory.Min,
						Max: v2Policy.Memory.Max,
					},
					Step: v2Policy.Memory.Step,
					PerCore: v1alpha3.SizingPolicyMemoryPerCore{
						MemoryMinMax: v1alpha3.MemoryMinMax{
							Min: v2Policy.Memory.PerCore.Min,
							Max: v2Policy.Memory.PerCore.Max,
						},
					},
				}
			}

			if v2Policy.Cores != nil {
				v3Policy.Cores = &v1alpha3.SizingPolicyCores{
					Min:  v2Policy.Cores.Min,
					Max:  v2Policy.Cores.Max,
					Step: v2Policy.Cores.Step,
				}
			}

			if len(v2Policy.CoreFractions) > 0 {
				v3Policy.CoreFractions = make([]v1alpha3.CoreFractionValue, len(v2Policy.CoreFractions))
				for j, v2Fraction := range v2Policy.CoreFractions {
					v3Policy.CoreFractions[j] = v1alpha3.CoreFractionValue(fmt.Sprintf("%d%%", v2Fraction))
				}
			}

			v3Spec.SizingPolicies[i] = v3Policy
		}
	}

	return v3Spec
}

func convertSpecV3ToV2(v3Spec v1alpha3.VirtualMachineClassSpec) (VirtualMachineClassSpec, error) {
	v2Spec := VirtualMachineClassSpec{
		NodeSelector: NodeSelector{
			MatchLabels:      v3Spec.NodeSelector.MatchLabels,
			MatchExpressions: v3Spec.NodeSelector.MatchExpressions,
		},
		Tolerations: v3Spec.Tolerations,
		CPU: CPU{
			Type:     CPUType(v3Spec.CPU.Type),
			Model:    v3Spec.CPU.Model,
			Features: v3Spec.CPU.Features,
			Discovery: CpuDiscovery{
				NodeSelector: v3Spec.CPU.Discovery.NodeSelector,
			},
		},
	}

	if len(v3Spec.SizingPolicies) > 0 {
		v2Spec.SizingPolicies = make([]SizingPolicy, len(v3Spec.SizingPolicies))
		for i, v3Policy := range v3Spec.SizingPolicies {
			v2Policy := SizingPolicy{
				DedicatedCores: v3Policy.DedicatedCores,
			}

			if v3Policy.Memory != nil {
				v2Policy.Memory = &SizingPolicyMemory{
					MemoryMinMax: MemoryMinMax{
						Min: v3Policy.Memory.Min,
						Max: v3Policy.Memory.Max,
					},
					Step: v3Policy.Memory.Step,
					PerCore: SizingPolicyMemoryPerCore{
						MemoryMinMax: MemoryMinMax{
							Min: v3Policy.Memory.PerCore.Min,
							Max: v3Policy.Memory.PerCore.Max,
						},
					},
				}
			}

			if v3Policy.Cores != nil {
				v2Policy.Cores = &SizingPolicyCores{
					Min:  v3Policy.Cores.Min,
					Max:  v3Policy.Cores.Max,
					Step: v3Policy.Cores.Step,
				}
			}

			if len(v3Policy.CoreFractions) > 0 {
				v2Policy.CoreFractions = make([]CoreFractionValue, len(v3Policy.CoreFractions))
				for j, v3Fraction := range v3Policy.CoreFractions {
					fractionStr := string(v3Fraction)
					if len(fractionStr) > 0 && fractionStr[len(fractionStr)-1] == '%' {
						fractionStr = fractionStr[:len(fractionStr)-1]
					}
					fractionInt, err := strconv.Atoi(fractionStr)
					if err != nil {
						return VirtualMachineClassSpec{}, fmt.Errorf("failed to parse core fraction: %w", err)
					}
					v2Policy.CoreFractions[j] = CoreFractionValue(fractionInt)
				}
			}

			v2Spec.SizingPolicies[i] = v2Policy
		}
	}

	return v2Spec, nil
}

func convertStatusV2ToV3(v2Status VirtualMachineClassStatus) v1alpha3.VirtualMachineClassStatus {
	return v1alpha3.VirtualMachineClassStatus{
		Phase: v1alpha3.VirtualMachineClassPhase(v2Status.Phase),
		CpuFeatures: v1alpha3.CpuFeatures{
			Enabled:          v2Status.CpuFeatures.Enabled,
			NotEnabledCommon: v2Status.CpuFeatures.NotEnabledCommon,
		},
		AvailableNodes:          v2Status.AvailableNodes,
		MaxAllocatableResources: v2Status.MaxAllocatableResources,
		Conditions:              v2Status.Conditions,
		ObservedGeneration:      v2Status.ObservedGeneration,
	}
}

func convertStatusV3ToV2(v3Status v1alpha3.VirtualMachineClassStatus) VirtualMachineClassStatus {
	return VirtualMachineClassStatus{
		Phase: VirtualMachineClassPhase(v3Status.Phase),
		CpuFeatures: CpuFeatures{
			Enabled:          v3Status.CpuFeatures.Enabled,
			NotEnabledCommon: v3Status.CpuFeatures.NotEnabledCommon,
		},
		AvailableNodes:          v3Status.AvailableNodes,
		MaxAllocatableResources: v3Status.MaxAllocatableResources,
		Conditions:              v3Status.Conditions,
		ObservedGeneration:      v3Status.ObservedGeneration,
	}
}
