/*
Copyright 2024 Flant JSC
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

package vmclass

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha3"
)

type ConversionHandler struct{}

func NewConversionHandler() *ConversionHandler {
	return &ConversionHandler{}
}

func (h *ConversionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var conversionReview apiextensionsv1.ConversionReview
	if err := json.Unmarshal(body, &conversionReview); err != nil {
		http.Error(w, fmt.Sprintf("failed to unmarshal request: %v", err), http.StatusBadRequest)
		return
	}

	response, err := h.Handle(&conversionReview)
	if err != nil {
		http.Error(w, fmt.Sprintf("conversion failed: %v", err), http.StatusInternalServerError)
		return
	}

	responseBody, err := json.Marshal(response)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to marshal response: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(responseBody)
}

func (h *ConversionHandler) Handle(conversionReview *apiextensionsv1.ConversionReview) (*apiextensionsv1.ConversionReview, error) {
	if conversionReview.Request == nil {
		return nil, fmt.Errorf("conversion request is nil")
	}

	response := &apiextensionsv1.ConversionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "ConversionReview",
		},
		Response: &apiextensionsv1.ConversionResponse{
			UID:    conversionReview.Request.UID,
			Result: metav1.Status{Status: "Success"},
		},
	}

	convertedObjects := make([]runtime.RawExtension, 0, len(conversionReview.Request.Objects))
	for _, obj := range conversionReview.Request.Objects {
		convertedObj, err := h.convertObject(obj, conversionReview.Request.DesiredAPIVersion)
		if err != nil {
			response.Response.Result = metav1.Status{
				Status:  "Failure",
				Message: err.Error(),
			}
			return response, nil
		}
		convertedObjects = append(convertedObjects, runtime.RawExtension{Raw: convertedObj})
	}

	response.Response.ConvertedObjects = convertedObjects
	return response, nil
}

func (h *ConversionHandler) convertObject(obj runtime.RawExtension, desiredAPIVersion string) ([]byte, error) {
	var typeMeta metav1.TypeMeta
	if err := json.Unmarshal(obj.Raw, &typeMeta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal TypeMeta: %w", err)
	}

	switch typeMeta.APIVersion {
	case "virtualization.deckhouse.io/v1alpha2":
		if desiredAPIVersion == "virtualization.deckhouse.io/v1alpha3" {
			return h.convertV1alpha2ToV1alpha3(obj.Raw)
		}
	case "virtualization.deckhouse.io/v1alpha3":
		if desiredAPIVersion == "virtualization.deckhouse.io/v1alpha2" {
			return h.convertV1alpha3ToV1alpha2(obj.Raw)
		}
	}

	return obj.Raw, nil
}

func (h *ConversionHandler) convertV1alpha2ToV1alpha3(data []byte) ([]byte, error) {
	var v2Class v1alpha2.VirtualMachineClass
	if err := json.Unmarshal(data, &v2Class); err != nil {
		return nil, fmt.Errorf("failed to unmarshal v1alpha2 VirtualMachineClass: %w", err)
	}

	v3Class := v1alpha3.VirtualMachineClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "virtualization.deckhouse.io/v1alpha3",
			Kind:       "VirtualMachineClass",
		},
		ObjectMeta: v2Class.ObjectMeta,
		Spec:       convertSpecV2ToV3(v2Class.Spec),
		Status:     convertStatusV2ToV3(v2Class.Status),
	}

	return json.Marshal(v3Class)
}

func (h *ConversionHandler) convertV1alpha3ToV1alpha2(data []byte) ([]byte, error) {
	var v3Class v1alpha3.VirtualMachineClass
	if err := json.Unmarshal(data, &v3Class); err != nil {
		return nil, fmt.Errorf("failed to unmarshal v1alpha3 VirtualMachineClass: %w", err)
	}

	v2Class := v1alpha2.VirtualMachineClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "virtualization.deckhouse.io/v1alpha2",
			Kind:       "VirtualMachineClass",
		},
		ObjectMeta: v3Class.ObjectMeta,
		Spec:       convertSpecV3ToV2(v3Class.Spec),
		Status:     convertStatusV3ToV2(v3Class.Status),
	}

	return json.Marshal(v2Class)
}

func convertSpecV2ToV3(v2Spec v1alpha2.VirtualMachineClassSpec) v1alpha3.VirtualMachineClassSpec {
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

func convertSpecV3ToV2(v3Spec v1alpha3.VirtualMachineClassSpec) v1alpha2.VirtualMachineClassSpec {
	v2Spec := v1alpha2.VirtualMachineClassSpec{
		NodeSelector: v1alpha2.NodeSelector{
			MatchLabels:      v3Spec.NodeSelector.MatchLabels,
			MatchExpressions: v3Spec.NodeSelector.MatchExpressions,
		},
		Tolerations: v3Spec.Tolerations,
		CPU: v1alpha2.CPU{
			Type:     v1alpha2.CPUType(v3Spec.CPU.Type),
			Model:    v3Spec.CPU.Model,
			Features: v3Spec.CPU.Features,
			Discovery: v1alpha2.CpuDiscovery{
				NodeSelector: v3Spec.CPU.Discovery.NodeSelector,
			},
		},
	}

	if len(v3Spec.SizingPolicies) > 0 {
		v2Spec.SizingPolicies = make([]v1alpha2.SizingPolicy, len(v3Spec.SizingPolicies))
		for i, v3Policy := range v3Spec.SizingPolicies {
			v2Policy := v1alpha2.SizingPolicy{
				DedicatedCores: v3Policy.DedicatedCores,
			}

			if v3Policy.Memory != nil {
				v2Policy.Memory = &v1alpha2.SizingPolicyMemory{
					MemoryMinMax: v1alpha2.MemoryMinMax{
						Min: v3Policy.Memory.Min,
						Max: v3Policy.Memory.Max,
					},
					Step: v3Policy.Memory.Step,
					PerCore: v1alpha2.SizingPolicyMemoryPerCore{
						MemoryMinMax: v1alpha2.MemoryMinMax{
							Min: v3Policy.Memory.PerCore.Min,
							Max: v3Policy.Memory.PerCore.Max,
						},
					},
				}
			}

			if v3Policy.Cores != nil {
				v2Policy.Cores = &v1alpha2.SizingPolicyCores{
					Min:  v3Policy.Cores.Min,
					Max:  v3Policy.Cores.Max,
					Step: v3Policy.Cores.Step,
				}
			}

			if len(v3Policy.CoreFractions) > 0 {
				v2Policy.CoreFractions = make([]v1alpha2.CoreFractionValue, len(v3Policy.CoreFractions))
				for j, v3Fraction := range v3Policy.CoreFractions {
					fractionStr := string(v3Fraction)
					if len(fractionStr) > 0 && fractionStr[len(fractionStr)-1] == '%' {
						fractionStr = fractionStr[:len(fractionStr)-1]
					}
					fractionInt, err := strconv.Atoi(fractionStr)
					if err != nil {
						fractionInt = 100
					}
					v2Policy.CoreFractions[j] = v1alpha2.CoreFractionValue(fractionInt)
				}
			}

			v2Spec.SizingPolicies[i] = v2Policy
		}
	}

	return v2Spec
}

func convertStatusV2ToV3(v2Status v1alpha2.VirtualMachineClassStatus) v1alpha3.VirtualMachineClassStatus {
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

func convertStatusV3ToV2(v3Status v1alpha3.VirtualMachineClassStatus) v1alpha2.VirtualMachineClassStatus {
	return v1alpha2.VirtualMachineClassStatus{
		Phase: v1alpha2.VirtualMachineClassPhase(v3Status.Phase),
		CpuFeatures: v1alpha2.CpuFeatures{
			Enabled:          v3Status.CpuFeatures.Enabled,
			NotEnabledCommon: v3Status.CpuFeatures.NotEnabledCommon,
		},
		AvailableNodes:          v3Status.AvailableNodes,
		MaxAllocatableResources: v3Status.MaxAllocatableResources,
		Conditions:              v3Status.Conditions,
		ObservedGeneration:      v3Status.ObservedGeneration,
	}
}
