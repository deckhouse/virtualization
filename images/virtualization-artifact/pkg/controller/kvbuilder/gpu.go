/*
Copyright 2026 Flant JSC

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

package kvbuilder

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
)

const (
	GPUName                            = "gpu"
	GPUResourceClaimTemplateNameSuffix = "-gpu-template"
	GPUResourceClaimRequestName        = "req-gpu"
	AppliedGPUAnnotation               = "internal.virtualization.deckhouse.io/applied-gpu-id"
)

func GPUResourceClaimTemplateName(vmName string) string {
	return vmName + GPUResourceClaimTemplateNameSuffix
}

func (b *KVVM) SetGPU(vmName, gpuID string) {
	b.Resource.Spec.Template.Spec.ResourceClaims = slices.DeleteFunc(
		b.Resource.Spec.Template.Spec.ResourceClaims,
		func(claim virtv1.ResourceClaim) bool { return claim.Name == GPUName },
	)
	b.Resource.Spec.Template.Spec.Domain.Devices.GPUs = slices.DeleteFunc(
		b.Resource.Spec.Template.Spec.Domain.Devices.GPUs,
		func(gpu virtv1.GPU) bool { return gpu.Name == GPUName },
	)

	if gpuID == "" {
		if b.Resource.Annotations != nil {
			delete(b.Resource.Annotations, AppliedGPUAnnotation)
		}
		return
	}

	b.Resource.Spec.Template.Spec.ResourceClaims = append(b.Resource.Spec.Template.Spec.ResourceClaims, virtv1.ResourceClaim{
		PodResourceClaim: corev1.PodResourceClaim{
			Name:                      GPUName,
			ResourceClaimTemplateName: ptr.To(GPUResourceClaimTemplateName(vmName)),
		},
	})
	b.Resource.Spec.Template.Spec.Domain.Devices.GPUs = append(b.Resource.Spec.Template.Spec.Domain.Devices.GPUs, virtv1.GPU{
		Name: GPUName,
		ClaimRequest: &virtv1.ClaimRequest{
			ClaimName:   ptr.To(GPUName),
			RequestName: ptr.To(GPUResourceClaimRequestName),
		},
	})

	if b.Resource.Annotations == nil {
		b.Resource.Annotations = make(map[string]string, 1)
	}
	b.Resource.Annotations[AppliedGPUAnnotation] = gpuID
}
