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
	"fmt"
	"strconv"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

// ResolveSpeed determines USB hub type from speed in Mbps.
// https://mjmwired.net/kernel/Documentation/ABI/testing/sysfs-bus-usb#502
func ResolveSpeed(speed int) (isHS, isSS bool) {
	return speed == 480, speed >= 5000
}

// GetTotalPortsPerHub returns the number of ports per hub (total / 2).
func GetTotalPortsPerHub(nodeAnnotations map[string]string) (int, error) {
	totalPortsStr, exists := nodeAnnotations[annotations.AnnUSBIPTotalPorts]
	if !exists {
		return 0, fmt.Errorf("node does not have %s annotation", annotations.AnnUSBIPTotalPorts)
	}
	totalPorts, err := strconv.Atoi(totalPortsStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s annotation: %w", annotations.AnnUSBIPTotalPorts, err)
	}
	return totalPorts / 2, nil
}

// GetUsedPorts returns the number of used ports for the given hub type.
func GetUsedPorts(nodeAnnotations map[string]string, hubAnnotation string) (int, error) {
	usedPortsStr, exists := nodeAnnotations[hubAnnotation]
	if !exists {
		return 0, fmt.Errorf("node does not have %s annotation", hubAnnotation)
	}
	usedPorts, err := strconv.Atoi(usedPortsStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s annotation: %w", hubAnnotation, err)
	}
	return usedPorts, nil
}

// CheckFreePort checks if a node has free USBIP ports for the given speed.
// Returns true if there is at least one free port, false otherwise.
func CheckFreePort(nodeAnnotations map[string]string, speed int) (bool, error) {
	return CheckFreePortForRequest(nodeAnnotations, speed, 1)
}

// CheckFreePortForRequest checks if there are enough free ports for a specific request.
// It adds the requested count to the currently used ports and compares with total.
func CheckFreePortForRequest(nodeAnnotations map[string]string, speed, requestedCount int) (bool, error) {
	totalPortsPerHub, err := GetTotalPortsPerHub(nodeAnnotations)
	if err != nil {
		return false, err
	}

	usedPorts, err := getUsedPortsForSpeed(nodeAnnotations, speed)
	if err != nil {
		return false, err
	}

	return (usedPorts + requestedCount) <= totalPortsPerHub, nil
}
