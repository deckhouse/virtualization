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

const PCINamePrefix = "pci-"

func PCIResourceClaimTemplateName(deviceName string) string {
	return deviceName + "-template"
}

func PCIResourceClaimRequestName(deviceName string) string {
	return "req-" + deviceName
}

// SetPCIDevices attaches PCI devices through DRA resource claims: every device
// references the ResourceClaimTemplate owned by its PCIDevice resource, so the
// claim allocation both selects the exact device and pins the pod to its node.
func (b *KVVM) SetPCIDevices(devices []v1alpha2.PCIDeviceSpecRef) {
	devices = SortPCIDevices(devices)

	b.Resource.Spec.Template.Spec.ResourceClaims = slices.DeleteFunc(
		b.Resource.Spec.Template.Spec.ResourceClaims,
		func(claim virtv1.ResourceClaim) bool {
			return strings.HasPrefix(claim.Name, PCINamePrefix) &&
				claim.ResourceClaimTemplateName != nil &&
				*claim.ResourceClaimTemplateName == PCIResourceClaimTemplateName(claim.Name)
		},
	)
	b.Resource.Spec.Template.Spec.Domain.Devices.HostDevices = slices.DeleteFunc(
		b.Resource.Spec.Template.Spec.Domain.Devices.HostDevices,
		func(hostDevice virtv1.HostDevice) bool {
			return strings.HasPrefix(hostDevice.Name, PCINamePrefix) && hostDevice.ClaimRequest != nil
		},
	)

	for _, device := range devices {
		b.Resource.Spec.Template.Spec.ResourceClaims = append(b.Resource.Spec.Template.Spec.ResourceClaims, virtv1.ResourceClaim{
			PodResourceClaim: corev1.PodResourceClaim{
				Name:                      device.Name,
				ResourceClaimTemplateName: ptr.To(PCIResourceClaimTemplateName(device.Name)),
			},
		})
		b.Resource.Spec.Template.Spec.Domain.Devices.HostDevices = append(b.Resource.Spec.Template.Spec.Domain.Devices.HostDevices, virtv1.HostDevice{
			Name: device.Name,
			ClaimRequest: &virtv1.ClaimRequest{
				ClaimName:   ptr.To(device.Name),
				RequestName: ptr.To(PCIResourceClaimRequestName(device.Name)),
			},
		})
	}
}

// SortPCIDevices orders devices by name so that reordering the spec list is a no-op.
func SortPCIDevices(devices []v1alpha2.PCIDeviceSpecRef) []v1alpha2.PCIDeviceSpecRef {
	if len(devices) == 0 {
		return nil
	}
	sorted := slices.Clone(devices)
	slices.SortStableFunc(sorted, func(a, b v1alpha2.PCIDeviceSpecRef) int {
		return strings.Compare(a.Name, b.Name)
	})
	return sorted
}
