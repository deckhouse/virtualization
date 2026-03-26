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

package usb

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func CheckFreePortOnNodeExcludingLocalUSBs(ctx context.Context, cl client.Client, nodeName string, speed int) (bool, error) {
	return CheckFreePortForRequestOnNodeExcludingLocalUSBs(ctx, cl, nodeName, speed, 1)
}

func CheckFreePortForRequestOnNodeExcludingLocalUSBs(ctx context.Context, cl client.Client, nodeName string, speed, requestedCount int) (bool, error) {
	node := &corev1.Node{}
	if err := cl.Get(ctx, client.ObjectKey{Name: nodeName}, node); err != nil {
		return false, err
	}

	isHS, isSS := ResolveSpeed(speed)
	if !isHS && !isSS {
		return false, fmt.Errorf("unsupported USB speed: %d", speed)
	}

	totalPortsPerHub, err := GetTotalPortsPerHub(node.Annotations)
	if err != nil {
		return false, err
	}

	usedPorts, err := getUsedPortsForSpeed(node.Annotations, speed)
	if err != nil {
		return false, err
	}

	excludedLocalUSBs, err := countLocalAttachedUSBsOnNodeBySpeed(ctx, cl, nodeName, speed)
	if err != nil {
		return false, err
	}

	effectiveUsedPorts := usedPorts - excludedLocalUSBs
	if effectiveUsedPorts < 0 {
		effectiveUsedPorts = 0
	}

	return (effectiveUsedPorts + requestedCount) <= totalPortsPerHub, nil
}

func getUsedPortsForSpeed(nodeAnnotations map[string]string, speed int) (int, error) {
	isHS, isSS := ResolveSpeed(speed)

	switch {
	case isHS:
		return GetUsedPorts(nodeAnnotations, annotations.AnnUSBIPHighSpeedHubUsedPorts)
	case isSS:
		return GetUsedPorts(nodeAnnotations, annotations.AnnUSBIPSuperSpeedHubUsedPorts)
	default:
		return 0, fmt.Errorf("unsupported USB speed: %d", speed)
	}
}

func countLocalAttachedUSBsOnNodeBySpeed(ctx context.Context, cl client.Client, nodeName string, speed int) (int, error) {
	var vmList v1alpha2.VirtualMachineList
	if err := cl.List(ctx, &vmList, client.MatchingFields{indexer.IndexFieldVMByNode: nodeName}); err != nil {
		return 0, err
	}

	count := 0
	usbCache := make(map[client.ObjectKey]*v1alpha2.USBDevice)
	for i := range vmList.Items {
		vm := &vmList.Items[i]
		for _, usbStatus := range vm.Status.USBDevices {
			if !usbStatus.Attached {
				continue
			}

			key := client.ObjectKey{Name: usbStatus.Name, Namespace: vm.Namespace}
			usbDevice, ok := usbCache[key]
			if !ok {
				usbDevice = &v1alpha2.USBDevice{}
				if err := cl.Get(ctx, key, usbDevice); err != nil {
					return 0, err
				}
				usbCache[key] = usbDevice
			}

			if usbDevice.Status.NodeName != nodeName {
				continue
			}

			if sameSpeedClass(usbDevice.Status.Attributes.Speed, speed) {
				count++
			}
		}
	}

	return count, nil
}

func sameSpeedClass(deviceSpeed, requestedSpeed int) bool {
	deviceHS, deviceSS := ResolveSpeed(deviceSpeed)
	requestedHS, requestedSS := ResolveSpeed(requestedSpeed)

	return (deviceHS && requestedHS) || (deviceSS && requestedSS)
}
