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

package validators

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type AffinityValidator struct{}

func NewAffinityValidator() *AffinityValidator {
	return &AffinityValidator{}
}

func (v *AffinityValidator) ValidateCreate(_ context.Context, vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(vm)
}

func (v *AffinityValidator) ValidateUpdate(_ context.Context, _, newVM *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	return v.Validate(newVM)
}

func (v *AffinityValidator) Validate(vm *v1alpha2.VirtualMachine) (admission.Warnings, error) {
	var errs []error

	affinity := vm.Spec.Affinity

	if affinity != nil {
		if affinity.NodeAffinity != nil {
			errs = append(errs, v.ValidateNodeAffinity(affinity.NodeAffinity)...)
		}
		if affinity.VirtualMachineAndPodAffinity != nil {
			for _, term := range affinity.VirtualMachineAndPodAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
				for _, sr := range term.LabelSelector.MatchExpressions {
					errs = append(errs, v.ValidateLabelSelectorRequirement(sr)...)
				}

				for _, sr := range term.NamespaceSelector.MatchExpressions {
					errs = append(errs, v.ValidateLabelSelectorRequirement(sr)...)
				}
			}

			for _, term := range affinity.VirtualMachineAndPodAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
				if term.Weight < 0 || term.Weight > 100 {
					errs = append(errs, errors.New("terms weight must be in range [0, 100]"))
				}

				for _, sr := range term.VirtualMachineAndPodAffinityTerm.LabelSelector.MatchExpressions {
					errs = append(errs, v.ValidateLabelSelectorRequirement(sr)...)
				}

				for _, sr := range term.VirtualMachineAndPodAffinityTerm.NamespaceSelector.MatchExpressions {
					errs = append(errs, v.ValidateLabelSelectorRequirement(sr)...)
				}
			}
		}
		if affinity.VirtualMachineAndPodAntiAffinity != nil {
			for _, term := range affinity.VirtualMachineAndPodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
				for _, sr := range term.LabelSelector.MatchExpressions {
					errs = append(errs, v.ValidateLabelSelectorRequirement(sr)...)
				}

				for _, sr := range term.NamespaceSelector.MatchExpressions {
					errs = append(errs, v.ValidateLabelSelectorRequirement(sr)...)
				}
			}

			for _, term := range affinity.VirtualMachineAndPodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
				if term.Weight < 0 || term.Weight > 100 {
					errs = append(errs, errors.New("terms weight must be in range [0, 100]"))
				}

				for _, sr := range term.VirtualMachineAndPodAffinityTerm.LabelSelector.MatchExpressions {
					errs = append(errs, v.ValidateLabelSelectorRequirement(sr)...)
				}

				for _, sr := range term.VirtualMachineAndPodAffinityTerm.NamespaceSelector.MatchExpressions {
					errs = append(errs, v.ValidateLabelSelectorRequirement(sr)...)
				}
			}
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("errors while validating affinity: %w", errors.Join(errs...))
	}

	return nil, nil
}

func (v *AffinityValidator) ValidateNodeAffinity(na *corev1.NodeAffinity) []error {
	var errs []error

	if na.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		errs = append(errs, v.ValidateNodeSelector(na.RequiredDuringSchedulingIgnoredDuringExecution)...)
	}

	if len(na.PreferredDuringSchedulingIgnoredDuringExecution) > 0 {
		errs = append(errs, v.ValidatePreferredSchedulingTerms(na.PreferredDuringSchedulingIgnoredDuringExecution)...)
	}

	return errs
}

func (v *AffinityValidator) ValidateNodeSelector(nodeSelector *corev1.NodeSelector) []error {
	var errs []error

	if len(nodeSelector.NodeSelectorTerms) == 0 {
		errs = append(errs, errors.New("must have at least one node selector term"))
	}

	for _, term := range nodeSelector.NodeSelectorTerms {
		errs = append(errs, v.ValidateNodeSelectorTerm(term)...)
	}

	return errs
}

func (v *AffinityValidator) ValidatePreferredSchedulingTerms(terms []corev1.PreferredSchedulingTerm) []error {
	var errs []error

	for _, term := range terms {
		if term.Weight < 0 || term.Weight > 100 {
			errs = append(errs, errors.New("weight must be between 0 and 100"))
		}

		errs = append(errs, v.ValidateNodeSelectorTerm(term.Preference)...)
	}

	return errs
}

func (v *AffinityValidator) ValidateNodeSelectorTerm(term corev1.NodeSelectorTerm) []error {
	var errs []error

	for _, req := range term.MatchExpressions {
		errs = append(errs, v.ValidateNodeSelectorRequirement(req)...)
	}

	for _, req := range term.MatchFields {
		errs = append(errs, v.ValidateNodeFieldSelectorRequirement(req)...)
	}

	return errs
}

// ValidateNodeSelectorRequirement tests that the specified NodeSelectorRequirement fields has valid data
func (v *AffinityValidator) ValidateNodeSelectorRequirement(rq corev1.NodeSelectorRequirement) []error {
	var allErrs []error
	switch rq.Operator {
	case corev1.NodeSelectorOpIn, corev1.NodeSelectorOpNotIn:
		if len(rq.Values) == 0 {
			allErrs = append(allErrs, errors.New("must be specified when `operator` is 'In' or 'NotIn'"))
		}
	case corev1.NodeSelectorOpExists, corev1.NodeSelectorOpDoesNotExist:
		if len(rq.Values) > 0 {
			allErrs = append(allErrs, errors.New("may not be specified when `operator` is 'Exists' or 'DoesNotExist'"))
		}

	case corev1.NodeSelectorOpGt, corev1.NodeSelectorOpLt:
		if len(rq.Values) != 1 {
			allErrs = append(allErrs, errors.New("must be specified single value when `operator` is 'Lt' or 'Gt'"))
		}
	default:
		allErrs = append(allErrs, errors.New("not a valid selector operator"))
	}

	return allErrs
}

// ValidateNodeFieldSelectorRequirement tests that the specified NodeSelectorRequirement fields has valid data
func (v *AffinityValidator) ValidateNodeFieldSelectorRequirement(req corev1.NodeSelectorRequirement) []error {
	var allErrs []error

	switch req.Operator {
	case corev1.NodeSelectorOpIn, corev1.NodeSelectorOpNotIn:
		if len(req.Values) != 1 {
			allErrs = append(allErrs, errors.New("must be only one value when `operator` is 'In' or 'NotIn' for node field selector"))
		}
	default:
		allErrs = append(allErrs, errors.New("not a valid selector operator"))
	}

	return allErrs
}

func (v *AffinityValidator) ValidateLabelSelectorRequirement(sr metav1.LabelSelectorRequirement) []error {
	var allErrs []error
	switch sr.Operator {
	case metav1.LabelSelectorOpIn, metav1.LabelSelectorOpNotIn:
		if len(sr.Values) == 0 {
			allErrs = append(allErrs, errors.New("must be specified when `operator` is 'In' or 'NotIn'"))
		}
	case metav1.LabelSelectorOpExists, metav1.LabelSelectorOpDoesNotExist:
		if len(sr.Values) > 0 {
			allErrs = append(allErrs, errors.New("may not be specified when `operator` is 'Exists' or 'DoesNotExist'"))
		}
	default:
		allErrs = append(allErrs, errors.New("not a valid selector operator"))
	}

	return allErrs
}
