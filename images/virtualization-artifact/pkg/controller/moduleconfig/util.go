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

package moduleconfig

import (
	"fmt"
	"net/netip"

	corev1 "k8s.io/api/core/v1"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
)

const virtualMachineCIDRs = "virtualMachineCIDRs"

func isEqualCIDRs(a, b netip.Prefix) bool {
	return a.Addr() == b.Addr() && a.Bits() == b.Bits()
}

func checkOverlapsCIDRs(prefixes []netip.Prefix) error {
	for i := 0; i < len(prefixes); i++ {
		for j := i + 1; j < len(prefixes); j++ {
			if prefixes[i].Overlaps(prefixes[j]) {
				return fmt.Errorf("subnets must not overlap, but there is an overlap between CIDRs %v and %v. Please adjust the configuration to resolve this issue", prefixes[i], prefixes[j])
			}
		}
	}
	return nil
}

func checkNodeAddressesOverlap(nodes []corev1.Node, excludedPrefixes []netip.Prefix) error {
	for _, node := range nodes {
		for _, address := range node.Status.Addresses {
			if address.Type == corev1.NodeInternalIP || address.Type == corev1.NodeExternalIP {
				ip, err := netip.ParseAddr(address.Address)
				if err != nil {
					// No way to detect if invalid address is in excludedPrefixes, just ignore the error and move on.
					continue
				}

				for _, prefix := range excludedPrefixes {
					if prefix.Contains(ip) {
						return fmt.Errorf("subnet %s is invalid, it should not contain addresses assigned to nodes: got node %s with IP %s", prefix, node.GetName(), ip)
					}
				}
			}
		}
	}
	return nil
}

func parseCIDRs(settings mcapi.SettingsValues) ([]netip.Prefix, error) {
	raw := settings[virtualMachineCIDRs].([]interface{})
	CIDRs, err := convertToStringSlice(raw)
	if err != nil {
		return nil, err
	}
	parsedCIDRs := make([]netip.Prefix, len(CIDRs))
	for i, CIDR := range CIDRs {
		parsedCIDR, err := netip.ParsePrefix(CIDR)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CIDR %s: %w", CIDR, err)
		}
		parsedCIDRs[i] = parsedCIDR
	}
	return parsedCIDRs, nil
}

func convertToStringSlice(input []interface{}) ([]string, error) {
	result := make([]string, len(input))
	for i, v := range input {
		strVal, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("failed to convert value %v to a string", v)
		}
		result[i] = strVal
	}
	return result, nil
}
