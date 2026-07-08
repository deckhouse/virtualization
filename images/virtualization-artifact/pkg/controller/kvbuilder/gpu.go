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
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	GPUNamePrefix    = "gpu-"
	GPUDRADriverName = "gpu.deckhouse.io"
)

func GPUResourceClaimName(deviceName string) string {
	return GPUNamePrefix + deviceName
}

func GPUResourceClaimTemplateName(vmName, deviceName string) string {
	// The vmName hash suffix keeps the template name unique per VM: two VMs in the
	// same namespace whose "<vmName>-<deviceName>" prefixes collide (e.g. VM "a" with
	// device "b-c" and VM "a-b" with device "c") would otherwise fight over one name
	// and deadlock the losing VM's reconciliation on a not-controlled-by error.
	return vmName + "-" + deviceName + "-" + GenerateSerial(vmName)[:8]
}

func IsGPUResourceClaimTemplateName(vmName, templateName string) bool {
	return strings.HasPrefix(templateName, vmName+"-")
}

func (b *KVVM) SetGPUDevices(vmName string, devices []v1alpha2.GPUDeviceSpec) {
	devices = SortGPUDevices(devices)

	b.Resource.Spec.Template.Spec.ResourceClaims = slices.DeleteFunc(
		b.Resource.Spec.Template.Spec.ResourceClaims,
		func(claim virtv1.ResourceClaim) bool {
			return strings.HasPrefix(claim.Name, GPUNamePrefix)
		},
	)
	b.Resource.Spec.Template.Spec.Domain.Devices.GPUs = slices.DeleteFunc(
		b.Resource.Spec.Template.Spec.Domain.Devices.GPUs,
		func(gpu virtv1.GPU) bool {
			return strings.HasPrefix(gpu.Name, GPUNamePrefix)
		},
	)

	if len(devices) == 0 {
		return
	}

	for _, device := range devices {
		claimName := GPUResourceClaimName(device.Name)
		b.Resource.Spec.Template.Spec.ResourceClaims = append(b.Resource.Spec.Template.Spec.ResourceClaims, virtv1.ResourceClaim{
			PodResourceClaim: corev1.PodResourceClaim{
				Name:                      claimName,
				ResourceClaimTemplateName: ptr.To(GPUResourceClaimTemplateName(vmName, device.Name)),
			},
		})
		b.Resource.Spec.Template.Spec.Domain.Devices.GPUs = append(b.Resource.Spec.Template.Spec.Domain.Devices.GPUs, virtv1.GPU{
			Name: claimName,
			ClaimRequest: &virtv1.ClaimRequest{
				ClaimName:   ptr.To(claimName),
				RequestName: ptr.To(GPUResourceClaimName(device.Name)),
			},
		})
	}
}

func SortGPUDevices(devices []v1alpha2.GPUDeviceSpec) []v1alpha2.GPUDeviceSpec {
	if len(devices) == 0 {
		return nil
	}
	sorted := slices.Clone(devices)
	slices.SortFunc(sorted, func(a, b v1alpha2.GPUDeviceSpec) int {
		return strings.Compare(a.Name, b.Name)
	})
	return sorted
}
