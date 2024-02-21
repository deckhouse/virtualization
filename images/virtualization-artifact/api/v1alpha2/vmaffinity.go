package v1alpha2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VMAffinity struct {
	NodeAffinity                     *corev1.NodeAffinity              `json:"nodeAffinity,omitempty"`
	VirtualMachineAndPodAffinity     *VirtualMachineAndPodAffinity     `json:"virtualMachineAndPodAffinity,omitempty"`
	VirtualMachineAndPodAntiAffinity *VirtualMachineAndPodAntiAffinity `json:"virtualMachineAndPodAntiAffinity,omitempty"`
}

type VirtualMachineAndPodAffinity struct {
	RequiredDuringSchedulingIgnoredDuringExecution  []VirtualMachineAndPodAffinityTerm         `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`
	PreferredDuringSchedulingIgnoredDuringExecution []WeightedVirtualMachineAndPodAffinityTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

type VirtualMachineAndPodAntiAffinity struct {
	RequiredDuringSchedulingIgnoredDuringExecution  []VirtualMachineAndPodAffinityTerm         `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty"`
	PreferredDuringSchedulingIgnoredDuringExecution []WeightedVirtualMachineAndPodAffinityTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty"`
}

type WeightedVirtualMachineAndPodAffinityTerm struct {
	// weight associated with matching the corresponding vmAndPodAffinityTerm,
	// in the range 1-100.
	Weight int32 `json:"weight"`
	// Required. A vm affinity term, associated with the corresponding weight.
	VirtualMachineAndPodAffinityTerm VirtualMachineAndPodAffinityTerm `json:"virtualMachineAndPodAffinityTerm"`
}

type VirtualMachineAndPodAffinityTerm struct {
	LabelSelector     *metav1.LabelSelector `json:"labelSelector,omitempty"`
	Namespaces        []string              `json:"namespaces,omitempty"`
	TopologyKey       string                `json:"topologyKey"`
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

func NewAffinityFromVMAffinity(vmAffinity *VMAffinity) *corev1.Affinity {
	if vmAffinity == nil {
		return nil
	}
	aff := &corev1.Affinity{
		NodeAffinity: vmAffinity.NodeAffinity,
	}
	if vmAff := vmAffinity.VirtualMachineAndPodAffinity; vmAff != nil {
		corePodAff := &corev1.PodAffinity{}
		corePr := make([]corev1.WeightedPodAffinityTerm, len(vmAff.RequiredDuringSchedulingIgnoredDuringExecution))
		for i, pr := range vmAff.PreferredDuringSchedulingIgnoredDuringExecution {
			corePr[i] = corev1.WeightedPodAffinityTerm{
				Weight:          pr.Weight,
				PodAffinityTerm: corev1.PodAffinityTerm(pr.VirtualMachineAndPodAffinityTerm),
			}
		}
		coreRd := make([]corev1.PodAffinityTerm, len(vmAff.RequiredDuringSchedulingIgnoredDuringExecution))
		for i, rd := range vmAff.RequiredDuringSchedulingIgnoredDuringExecution {
			coreRd[i] = corev1.PodAffinityTerm(rd)
		}
		if len(corePr) > 0 {
			corePodAff.PreferredDuringSchedulingIgnoredDuringExecution = corePr
		}
		if len(coreRd) > 0 {
			corePodAff.RequiredDuringSchedulingIgnoredDuringExecution = coreRd
		}
		aff.PodAffinity = corePodAff
	}
	if vmAntiAff := vmAffinity.VirtualMachineAndPodAntiAffinity; vmAntiAff != nil {
		corePodAntiAff := &corev1.PodAntiAffinity{}
		corePr := make([]corev1.WeightedPodAffinityTerm, len(vmAntiAff.PreferredDuringSchedulingIgnoredDuringExecution))
		for i, pr := range vmAntiAff.PreferredDuringSchedulingIgnoredDuringExecution {
			corePr[i] = corev1.WeightedPodAffinityTerm{
				Weight:          pr.Weight,
				PodAffinityTerm: corev1.PodAffinityTerm(pr.VirtualMachineAndPodAffinityTerm),
			}
		}
		coreRd := make([]corev1.PodAffinityTerm, len(vmAntiAff.RequiredDuringSchedulingIgnoredDuringExecution))
		for i, rd := range vmAntiAff.RequiredDuringSchedulingIgnoredDuringExecution {
			coreRd[i] = corev1.PodAffinityTerm(rd)
		}
		if len(corePr) > 0 {
			corePodAntiAff.PreferredDuringSchedulingIgnoredDuringExecution = corePr
		}
		if len(coreRd) > 0 {
			corePodAntiAff.RequiredDuringSchedulingIgnoredDuringExecution = coreRd
		}
		aff.PodAntiAffinity = corePodAntiAff
	}
	return aff
}
