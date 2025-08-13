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

func CheckCIDRsOverlap(cidrs []netip.Prefix) error {
	for i := 0; i < len(cidrs)-1; i++ {
		err := CheckCIDRsOverlapWithSubnet(cidrs[i+1:], cidrs[i])
		if err != nil {
			return fmt.Errorf("no overlaps allowed: %w", err)
		}
	}

	return nil
}

func CheckCIDRsOverlapWithNodeAddresses(cidrs []netip.Prefix, nodes []corev1.Node) error {
	for _, node := range nodes {
		for _, address := range node.Status.Addresses {
			if address.Type == corev1.NodeInternalIP || address.Type == corev1.NodeExternalIP {
				nodeIP, err := netip.ParseAddr(address.Address)
				if err != nil {
					// No way to detect if cidrs contain invalid address. Just ignore the error and move on.
					continue
				}

				err = CheckCIDRsOverlapWithAddress(cidrs, nodeIP)
				if err != nil {
					return fmt.Errorf("check node %s: %w", node.GetName(), err)
				}
			}
		}
	}
	return nil
}

func CheckCIDRsOverlapWithAddress(cidrs []netip.Prefix, address netip.Addr) error {
	for _, cidr := range cidrs {
		if cidr.Contains(address) {
			return fmt.Errorf("subnet %v is invalid: should not contain address %v. Please adjust the configuration to resolve this issue", cidr, address)
		}
	}
	return nil
}

func CheckCIDRsOverlapWithSubnet(cidrs []netip.Prefix, subnet netip.Prefix) error {
	for _, cidr := range cidrs {
		if cidr.Overlaps(subnet) {
			return fmt.Errorf("subnet %v is invalid: should not overlap with subnet %v. Please adjust the configuration to resolve this issue", cidr, subnet)
		}
	}
	return nil
}

func CheckCIDRsOverlapWithPodSubnet(cidrs []netip.Prefix, subnet netip.Prefix) error {
	err := CheckCIDRsOverlapWithSubnet(cidrs, subnet)
	if err != nil {
		return fmt.Errorf("check overlap with global podSubnetCIDR: %w", err)
	}
	return nil
}

func CheckCIDRsOverlapWithServiceSubnet(cidrs []netip.Prefix, subnet netip.Prefix) error {
	err := CheckCIDRsOverlapWithSubnet(cidrs, subnet)
	if err != nil {
		return fmt.Errorf("check overlap with global serviceSubnetCIDR: %w", err)
	}
	return nil
}

func ParseCIDRs(settings mcapi.SettingsValues) ([]netip.Prefix, error) {
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
