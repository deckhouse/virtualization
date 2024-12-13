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

package mac

import (
	"strings"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const macPrefix = "mac-"

type AllocatedMACs map[string]*virtv2.VirtualMachineMACAddressLease

// MACAddressToLeaseName generate the Virtual Machine MAC Address Lease's name from the MAC address
func MACAddressToLeaseName(address string) string {
	//addr := net.ParseIP(ip)
	//if addr.To4() != nil {
	//	// IPv4 address
	//	return macPrefix + strings.ReplaceAll(addr.String(), ".", "-")
	//}

	// TODO dlopatin
	return macPrefix + "-test-test"
}

// LeaseNameToMACAddress generate the MAC address from the Virtual Machine MAC Address Lease's name
func LeaseNameToMACAddress(leaseName string) string {
	if strings.HasPrefix(leaseName, macPrefix) && len(leaseName) > len(macPrefix) {
		return strings.ReplaceAll(leaseName[len(macPrefix):], "-", ".")
	}

	return ""
}
