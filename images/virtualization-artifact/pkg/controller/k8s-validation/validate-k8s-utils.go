/*
Copyright 2014 The Kubernetes Authors.
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

/*

The code here is taken from https://github.com/kubernetes/kubernetes/blob/29fb8e8b5a41b2a7d760190284bae7f2829312d3/pkg/apis/core/validation/validation.go#L3288
The "core" package is change to exported package "k8s.io/api/core/v1" in
order to avoid dependency on kubernetes/kubernetes

https://github.com/kubernetes/kubernetes/blame/29fb8e8b5a41b2a7d760190284bae7f2829312d3/pkg/apis/core/validation/validation.go#L3288
The code is very stable, it was barely changed during the past years.
It makes it easier to copy and maintain instead of vendoring the whole kubernetes as module or
creating dry runs of the pod object during admission validation.
*/

package k8s_validation //nolint:stylecheck,nolintlint

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	unversionedvalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const isNotPositiveErrorMsg string = `must be greater than zero`

// ValidateNamespaceName can be used to check whether the given namespace name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidateNamespaceName = apimachineryvalidation.ValidateNamespaceName

// ValidateNodeName can be used to check whether the given node name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidateNodeName = apimachineryvalidation.NameIsDNSSubdomain

var nodeFieldSelectorValidators = map[string]func(string, bool) []string{
	metav1.ObjectNameField: ValidateNodeName,
}

// ValidateAffinity checks if given affinities are valid
func ValidateAffinity(affinity *v1alpha2.VMAffinity, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if affinity != nil {
		if affinity.NodeAffinity != nil {
			allErrs = append(allErrs, validateNodeAffinity(affinity.NodeAffinity, fldPath.Child("nodeAffinity"))...)
		}
		if affinity.VirtualMachineAndPodAffinity != nil {
			allErrs = append(allErrs, validatePodAffinity(affinity.VirtualMachineAndPodAffinity, fldPath.Child("virtualMachineAndPodAffinity"))...)
		}
		if affinity.VirtualMachineAndPodAntiAffinity != nil {
			allErrs = append(allErrs, validatePodAntiAffinity(affinity.VirtualMachineAndPodAntiAffinity, fldPath.Child("virtualMachineAndPodAntiAffinity"))...)
		}
	}

	return allErrs
}

// validateNodeAffinity tests that the specified nodeAffinity fields have valid data
func validateNodeAffinity(na *corev1.NodeAffinity, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	// TODO: Uncomment the next three lines once RequiredDuringSchedulingRequiredDuringExecution is implemented.
	// if na.RequiredDuringSchedulingRequiredDuringExecution != nil {
	//	allErrs = append(allErrs, ValidateNodeSelector(na.RequiredDuringSchedulingRequiredDuringExecution, fldPath.Child("requiredDuringSchedulingRequiredDuringExecution"))...)
	// }
	if na.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		allErrs = append(allErrs, ValidateNodeSelector(na.RequiredDuringSchedulingIgnoredDuringExecution, fldPath.Child("requiredDuringSchedulingIgnoredDuringExecution"))...)
	}
	if len(na.PreferredDuringSchedulingIgnoredDuringExecution) > 0 {
		allErrs = append(allErrs, ValidatePreferredSchedulingTerms(na.PreferredDuringSchedulingIgnoredDuringExecution, fldPath.Child("preferredDuringSchedulingIgnoredDuringExecution"))...)
	}
	return allErrs
}

// ValidatePreferredSchedulingTerms tests that the specified SoftNodeAffinity fields has valid data
func ValidatePreferredSchedulingTerms(terms []corev1.PreferredSchedulingTerm, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for i, term := range terms {
		if term.Weight <= 0 || term.Weight > 100 {
			allErrs = append(allErrs, field.Invalid(fldPath.Index(i).Child("weight"), term.Weight, "must be in the range 1-100"))
		}

		allErrs = append(allErrs, ValidateNodeSelectorTerm(term.Preference, fldPath.Index(i).Child("preference"))...)
	}
	return allErrs
}

// validatePodAffinity tests that the specified podAffinity fields have valid data
func validatePodAffinity(virtualMachineAndPodAffinity *v1alpha2.VirtualMachineAndPodAffinity, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	// TODO:Uncomment below code once RequiredDuringSchedulingRequiredDuringExecution is implemented.
	// if podAffinity.RequiredDuringSchedulingRequiredDuringExecution != nil {
	//	allErrs = append(allErrs, validatePodAffinityTerms(podAffinity.RequiredDuringSchedulingRequiredDuringExecution, false,
	//		fldPath.Child("requiredDuringSchedulingRequiredDuringExecution"))...)
	//}
	if virtualMachineAndPodAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		allErrs = append(allErrs, validatePodAffinityTerms(virtualMachineAndPodAffinity.RequiredDuringSchedulingIgnoredDuringExecution,
			fldPath.Child("requiredDuringSchedulingIgnoredDuringExecution"))...)
	}
	if virtualMachineAndPodAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
		allErrs = append(allErrs, validateWeightedPodAffinityTerms(virtualMachineAndPodAffinity.PreferredDuringSchedulingIgnoredDuringExecution,
			fldPath.Child("preferredDuringSchedulingIgnoredDuringExecution"))...)
	}
	return allErrs
}

// validateWeightedPodAffinityTerms tests that the specified weightedPodAffinityTerms fields have valid data
func validateWeightedPodAffinityTerms(weightedPodAffinityTerms []v1alpha2.WeightedVirtualMachineAndPodAffinityTerm, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for j, weightedTerm := range weightedPodAffinityTerms {
		if weightedTerm.Weight <= 0 || weightedTerm.Weight > 100 {
			allErrs = append(allErrs, field.Invalid(fldPath.Index(j).Child("weight"), weightedTerm.Weight, "must be in the range 1-100"))
		}
		allErrs = append(allErrs, validatePodAffinityTerm(weightedTerm.VirtualMachineAndPodAffinityTerm, fldPath.Index(j).Child("virtualMachineAndPodAffinityTerm"))...)
	}
	return allErrs
}

// validatePodAffinityTerm tests that the specified podAffinityTerm fields have valid data
func validatePodAffinityTerm(podAffinityTerm v1alpha2.VirtualMachineAndPodAffinityTerm, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, unversionedvalidation.ValidateLabelSelector(podAffinityTerm.LabelSelector, unversionedvalidation.LabelSelectorValidationOptions{}, fldPath.Child("labelSelector"))...)
	allErrs = append(allErrs, unversionedvalidation.ValidateLabelSelector(podAffinityTerm.NamespaceSelector, unversionedvalidation.LabelSelectorValidationOptions{}, fldPath.Child("namespaceSelector"))...)

	for _, name := range podAffinityTerm.Namespaces {
		for _, msg := range ValidateNamespaceName(name, false) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("namespace"), name, msg))
		}
	}
	if len(podAffinityTerm.TopologyKey) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("topologyKey"), "can not be empty"))
	}
	return append(allErrs, unversionedvalidation.ValidateLabelName(podAffinityTerm.TopologyKey, fldPath.Child("topologyKey"))...)
}

// validatePodAntiAffinity tests that the specified podAntiAffinity fields have valid data
func validatePodAntiAffinity(podAntiAffinity *v1alpha2.VirtualMachineAndPodAntiAffinity, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	// TODO:Uncomment below code once RequiredDuringSchedulingRequiredDuringExecution is implemented.
	// if podAntiAffinity.RequiredDuringSchedulingRequiredDuringExecution != nil {
	//	allErrs = append(allErrs, validatePodAffinityTerms(podAntiAffinity.RequiredDuringSchedulingRequiredDuringExecution, false,
	//		fldPath.Child("requiredDuringSchedulingRequiredDuringExecution"))...)
	//}
	if podAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		allErrs = append(allErrs, validatePodAffinityTerms(podAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution,
			fldPath.Child("requiredDuringSchedulingIgnoredDuringExecution"))...)
	}
	if podAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution != nil {
		allErrs = append(allErrs, validateWeightedPodAffinityTerms(podAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution,
			fldPath.Child("preferredDuringSchedulingIgnoredDuringExecution"))...)
	}
	return allErrs
}

// ValidateNodeSelector tests that the specified nodeSelector fields has valid data
func ValidateNodeSelector(nodeSelector *corev1.NodeSelector, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	termFldPath := fldPath.Child("nodeSelectorTerms")
	if len(nodeSelector.NodeSelectorTerms) == 0 {
		return append(allErrs, field.Required(termFldPath, "must have at least one node selector term"))
	}

	for i, term := range nodeSelector.NodeSelectorTerms {
		allErrs = append(allErrs, ValidateNodeSelectorTerm(term, termFldPath.Index(i))...)
	}

	return allErrs
}

// validatePodAffinityTerms tests that the specified podAffinityTerms fields have valid data
func validatePodAffinityTerms(podAffinityTerms []v1alpha2.VirtualMachineAndPodAffinityTerm, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for i, podAffinityTerm := range podAffinityTerms {
		allErrs = append(allErrs, validatePodAffinityTerm(podAffinityTerm, fldPath.Index(i))...)
	}
	return allErrs
}

// ValidateNodeSelectorTerm tests that the specified node selector term has valid data
func ValidateNodeSelectorTerm(term corev1.NodeSelectorTerm, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for j, req := range term.MatchExpressions {
		allErrs = append(allErrs, ValidateNodeSelectorRequirement(req, fldPath.Child("matchExpressions").Index(j))...)
	}

	for j, req := range term.MatchFields {
		allErrs = append(allErrs, ValidateNodeFieldSelectorRequirement(req, fldPath.Child("matchFields").Index(j))...)
	}

	return allErrs
}

// ValidateNodeSelectorRequirement tests that the specified NodeSelectorRequirement fields has valid data
func ValidateNodeSelectorRequirement(rq corev1.NodeSelectorRequirement, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	switch rq.Operator {
	case corev1.NodeSelectorOpIn, corev1.NodeSelectorOpNotIn:
		if len(rq.Values) == 0 {
			allErrs = append(allErrs, field.Required(fldPath.Child("values"), "must be specified when `operator` is 'In' or 'NotIn'"))
		}
	case corev1.NodeSelectorOpExists, corev1.NodeSelectorOpDoesNotExist:
		if len(rq.Values) > 0 {
			allErrs = append(allErrs, field.Forbidden(fldPath.Child("values"), "may not be specified when `operator` is 'Exists' or 'DoesNotExist'"))
		}

	case corev1.NodeSelectorOpGt, corev1.NodeSelectorOpLt:
		if len(rq.Values) != 1 {
			allErrs = append(allErrs, field.Required(fldPath.Child("values"), "must be specified single value when `operator` is 'Lt' or 'Gt'"))
		}
	default:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("operator"), rq.Operator, "not a valid selector operator"))
	}

	allErrs = append(allErrs, unversionedvalidation.ValidateLabelName(rq.Key, fldPath.Child("key"))...)

	return allErrs
}

// ValidateNodeFieldSelectorRequirement tests that the specified NodeSelectorRequirement fields has valid data
func ValidateNodeFieldSelectorRequirement(req corev1.NodeSelectorRequirement, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	switch req.Operator {
	case corev1.NodeSelectorOpIn, corev1.NodeSelectorOpNotIn:
		if len(req.Values) != 1 {
			allErrs = append(allErrs, field.Required(fldPath.Child("values"),
				"must be only one value when `operator` is 'In' or 'NotIn' for node field selector"))
		}
	default:
		allErrs = append(allErrs, field.Invalid(fldPath.Child("operator"), req.Operator, "not a valid selector operator"))
	}

	if vf, found := nodeFieldSelectorValidators[req.Key]; !found {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("key"), req.Key, "not a valid field selector key"))
	} else {
		for i, v := range req.Values {
			for _, msg := range vf(v, false) {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("values").Index(i), v, msg))
			}
		}
	}

	return allErrs
}

var supportedScheduleActions = sets.NewString(string(corev1.DoNotSchedule), string(corev1.ScheduleAnyway))

// ValidateTopologySpreadConstraints validates given TopologySpreadConstraints.
func ValidateTopologySpreadConstraints(constraints []corev1.TopologySpreadConstraint, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for i, constraint := range constraints {
		subFldPath := fldPath.Index(i)
		if err := ValidateMaxSkew(subFldPath.Child("maxSkew"), constraint.MaxSkew); err != nil {
			allErrs = append(allErrs, err)
		}
		if errs := ValidateTopologyKey(subFldPath.Child("topologyKey"), constraint.TopologyKey); errs != nil {
			allErrs = append(allErrs, errs...)
		}
		if err := ValidateWhenUnsatisfiable(subFldPath.Child("whenUnsatisfiable"), constraint.WhenUnsatisfiable); err != nil {
			allErrs = append(allErrs, err)
		}

		// this is missing in upstream codebase https://github.com/kubernetes/kubernetes/blob/master/pkg/apis/core/validation/validation.go#L6571-L6600
		// issue captured here https://github.com/kubernetes/kubernetes/issues/111791#issuecomment-1211184962
		allErrs = append(allErrs, unversionedvalidation.ValidateLabelSelector(constraint.LabelSelector, unversionedvalidation.LabelSelectorValidationOptions{}, fldPath.Child("labelSelector"))...)

		// tuple {topologyKey, whenUnsatisfiable} denotes one kind of spread constraint
		if err := ValidateSpreadConstraintNotRepeat(subFldPath.Child("{topologyKey, whenUnsatisfiable}"), constraint, constraints[i+1:]); err != nil {
			allErrs = append(allErrs, err)
		}
	}

	return allErrs
}

// ValidateMaxSkew tests that the argument is a valid MaxSkew.
func ValidateMaxSkew(fldPath *field.Path, maxSkew int32) *field.Error {
	if maxSkew <= 0 {
		return field.Invalid(fldPath, maxSkew, isNotPositiveErrorMsg)
	}
	return nil
}

// ValidateTopologyKey tests that the argument is a valid TopologyKey.
func ValidateTopologyKey(fldPath *field.Path, topologyKey string) field.ErrorList {
	allErrs := field.ErrorList{}
	if len(topologyKey) == 0 {
		return append(allErrs, field.Required(fldPath, "can not be empty"))
	}
	return unversionedvalidation.ValidateLabelName(topologyKey, fldPath)
}

// ValidateWhenUnsatisfiable tests that the argument is a valid UnsatisfiableConstraintAction.
func ValidateWhenUnsatisfiable(fldPath *field.Path, action corev1.UnsatisfiableConstraintAction) *field.Error {
	if !supportedScheduleActions.Has(string(action)) {
		return field.NotSupported(fldPath, action, supportedScheduleActions.List())
	}
	return nil
}

// ValidateSpreadConstraintNotRepeat tests that if `constraint` duplicates with `existingConstraintPairs`
// on TopologyKey and WhenUnsatisfiable fields.
func ValidateSpreadConstraintNotRepeat(fldPath *field.Path, constraint corev1.TopologySpreadConstraint, restingConstraints []corev1.TopologySpreadConstraint) *field.Error {
	for _, restingConstraint := range restingConstraints {
		if constraint.TopologyKey == restingConstraint.TopologyKey &&
			constraint.WhenUnsatisfiable == restingConstraint.WhenUnsatisfiable {
			return field.Duplicate(fldPath, fmt.Sprintf("{%v, %v}", constraint.TopologyKey, constraint.WhenUnsatisfiable))
		}
	}
	return nil
}
