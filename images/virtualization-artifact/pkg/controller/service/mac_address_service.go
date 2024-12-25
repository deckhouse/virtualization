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
	"net/netip"

	"github.com/deckhouse/deckhouse/pkg/log"
	k8snet "k8s.io/utils/net"

	"github.com/deckhouse/virtualization-controller/pkg/common/mac"
)

type MACAddressService struct {
	parsedCIDRs []netip.Prefix
}

func NewMACAddressService(
	logger *log.Logger,
	virtualMachineCIDRs []string,
) *MACAddressService {
	parsedCIDRs := make([]netip.Prefix, len(virtualMachineCIDRs))

	for i, cidr := range virtualMachineCIDRs {
		parsedCIDR, err := netip.ParsePrefix(cidr)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to parse CIDR %s:", cidr), "err", err)
			return nil
		}
		parsedCIDRs[i] = parsedCIDR
	}

	return &MACAddressService{
		parsedCIDRs: parsedCIDRs,
	}
}

func (s MACAddressService) IsAvailableAddress(address string, allocatedMACs mac.AllocatedMACs) error {
	// TODO dlopatin
	ip, err := netip.ParseAddr(address)
	if err != nil || !ip.IsValid() {
		return errors.New("invalid IP address format")
	}

	if _, ok := allocatedMACs[ip.String()]; ok {
		// already exists
		return ErrIPAddressAlreadyExist
	}

	for _, cidr := range s.parsedCIDRs {
		isFirstLast, err := isFirstLastIP(ip, cidr)
		if err != nil {
			return err
		}
		if !isFirstLast {
			if cidr.Contains(ip) {
				// available
				return nil
			}
		}
	}

	// out of range
	return ErrIPAddressOutOfRange
}

func (s MACAddressService) AllocateNewAddress(allocatedMACs mac.AllocatedMACs) (string, error) {
	// TODO dlopatin
	for _, cidr := range s.parsedCIDRs {
		for ip := cidr.Addr(); cidr.Contains(ip); ip = ip.Next() {
			if k8snet.RangeSize(toIPNet(cidr)) != 1 {
				isFirstLast, err := isFirstLastIP(ip, cidr)
				if err != nil {
					return "", err
				}
				if isFirstLast {
					continue
				}
			}

			if _, ok := allocatedMACs[ip.String()]; !ok {
				return ip.String(), nil
			}
		}
	}
	return "", errors.New("no remaining ips")
}
