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

package service

import (
	"errors"
	"fmt"
	"net"

	"github.com/go-logr/logr"
	k8snet "k8s.io/utils/net"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
)

type IpAddressService struct {
	logger      logr.Logger
	ParsedCIDRs []*net.IPNet
}

func NewIpAddressService(
	logger logr.Logger,
	virtualMachineCIDRs []string,
) *IpAddressService {
	parsedCIDRs := make([]*net.IPNet, len(virtualMachineCIDRs))

	for i, cidr := range virtualMachineCIDRs {
		_, parsedCIDR, err := net.ParseCIDR(cidr)
		if err != nil || parsedCIDR == nil {
			logger.Error(err, fmt.Sprintf("failed to parse CIDR %s:", cidr), "err", err)
			return nil
		}
		parsedCIDRs[i] = parsedCIDR
	}

	return &IpAddressService{
		logger:      logger.WithValues("service", "IpAddressService"),
		ParsedCIDRs: parsedCIDRs,
	}
}

func (s IpAddressService) IsAvailableAddress(address string, allocatedIPs common.AllocatedIPs) bool {
	ip := net.ParseIP(address)

	if _, ok := allocatedIPs[ip.String()]; !ok {
		for _, cidr := range s.ParsedCIDRs {
			if cidr.Contains(ip) {
				// available
				return true
			}
		}
		// out of range
		return false
	}
	// already exists
	return false
}

func (s IpAddressService) AllocateNewIP(allocatedIPs common.AllocatedIPs) (string, error) {
	for _, cidr := range s.ParsedCIDRs {
		for ip := cidr.IP.Mask(cidr.Mask); cidr.Contains(ip); inc(ip) {
			// Allow allocation of IP address explicitly specified using a 32-bit mask.
			if k8snet.RangeSize(cidr) != 1 {
				// Skip the allocation of the first or last addresses within the CIDR range.
				isFirstLast, err := isFirstLastIP(ip, cidr)
				if err != nil {
					return "", err
				}

				if isFirstLast {
					continue
				}
			}

			_, ok := allocatedIPs[ip.String()]
			if !ok {
				return ip.String(), nil
			}
		}
	}
	return "", errors.New("no remaining ips")
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func isFirstLastIP(ip net.IP, cidr *net.IPNet) (bool, error) {
	size := int(k8snet.RangeSize(cidr))

	first, err := k8snet.GetIndexedIP(cidr, 0)
	if err != nil {
		return false, err
	}

	if first.Equal(ip) {
		return true, nil
	}

	last, err := k8snet.GetIndexedIP(cidr, size-1)
	if err != nil {
		return false, err
	}

	return last.Equal(ip), nil
}
