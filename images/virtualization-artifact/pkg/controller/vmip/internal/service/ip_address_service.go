/*
Copyright 2025 Flant JSC

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
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	k8snet "k8s.io/utils/net"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type IPAddressService struct {
	parsedCIDRs []netip.Prefix
	client      client.Client
	virtClient  kubeclient.Client
}

func NewIPAddressService(
	virtualMachineCIDRs []string,
	client client.Client,
	virtClient kubeclient.Client,
) (*IPAddressService, error) {
	parsedCIDRs := make([]netip.Prefix, len(virtualMachineCIDRs))

	for i, cidr := range virtualMachineCIDRs {
		parsedCIDR, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CIDR %q: %w", cidr, err)
		}
		parsedCIDRs[i] = parsedCIDR
	}

	return &IPAddressService{
		parsedCIDRs: parsedCIDRs,
		client:      client,
		virtClient:  virtClient,
	}, nil
}

func (s IPAddressService) IsInsideOfRange(address string) error {
	addr, err := netip.ParseAddr(address)
	if err != nil || !addr.IsValid() {
		return errors.New("invalid IP address format")
	}

	for _, cidr := range s.parsedCIDRs {
		var isFirstLast bool
		isFirstLast, err = isFirstLastIP(addr, cidr)
		if err != nil {
			return err
		}
		if !isFirstLast {
			if cidr.Contains(addr) {
				// available
				return nil
			}
		}
	}

	// out of range
	return ErrIPAddressOutOfRange
}

func (s IPAddressService) AllocateNewIP(allocatedIPs ip.AllocatedIPs) (string, error) {
	for _, cidr := range s.parsedCIDRs {
		for addr := cidr.Addr(); cidr.Contains(addr); addr = addr.Next() {
			if k8snet.RangeSize(toIPNet(cidr)) != 1 {
				isFirstLast, err := isFirstLastIP(addr, cidr)
				if err != nil {
					return "", err
				}
				if isFirstLast {
					continue
				}
			}

			if _, ok := allocatedIPs[addr.String()]; !ok {
				return addr.String(), nil
			}
		}
	}
	return "", errors.New("no remaining ips")
}

func (s IPAddressService) GetAllocatedIPs(ctx context.Context) (ip.AllocatedIPs, error) {
	var leases virtv2.VirtualMachineIPAddressLeaseList

	err := s.client.List(ctx, &leases)
	if err != nil {
		return nil, fmt.Errorf("error getting leases: %w", err)
	}

	allocatedIPs := make(ip.AllocatedIPs, len(leases.Items))
	for _, lease := range leases.Items {
		allocatedIPs[ip.LeaseNameToIP(lease.Name)] = struct{}{}
	}

	return allocatedIPs, nil
}

func (s IPAddressService) GetLease(ctx context.Context, vmip *virtv2.VirtualMachineIPAddress) (*virtv2.VirtualMachineIPAddressLease, error) {
	// The IP address cannot be changed for a vmip. Once it has been assigned, it will remain the same.
	ipAddress := getAssignedIPAddress(vmip)
	if ipAddress != "" {
		return s.getLeaseByIPAddress(ctx, ipAddress)
	}

	// Either the Lease hasn't been created yet, or the address hasn't been set yet.
	// We need to make sure the Lease doesn't exist in the cluster by searching for it by label.
	return s.getLeaseByLabel(ctx, vmip)
}

func (s IPAddressService) getLeaseByIPAddress(ctx context.Context, ipAddress string) (*virtv2.VirtualMachineIPAddressLease, error) {
	// 1. Trying to find the Lease in the local cache.
	lease, err := object.FetchObject(ctx, types.NamespacedName{Name: ip.IpToLeaseName(ipAddress)}, s.client, &virtv2.VirtualMachineIPAddressLease{})
	if err != nil {
		return nil, fmt.Errorf("fetch lease in local cache: %w", err)
	}

	if lease != nil {
		return lease, nil
	}

	// The local cache might be outdated, which is why the Lease is not present in the cache, even though it may already exist in the cluster.
	// Double-check Lease existence in the cluster by making a direct request to the Kubernetes API.
	lease, err = s.virtClient.VirtualMachineIPAddressLeases().Get(ctx, ip.IpToLeaseName(ipAddress), metav1.GetOptions{})
	switch {
	case err == nil:
		logger.FromContext(ctx).Warn("The lease was not found by ip address in the local cache, but it already exists in the cluster", "leaseName", lease.Name)
		return lease, nil
	case k8serrors.IsNotFound(err):
		return nil, nil
	default:
		return nil, fmt.Errorf("get lease via direct request to kubeapi: %w", err)
	}
}

func (s IPAddressService) getLeaseByLabel(ctx context.Context, vmip *virtv2.VirtualMachineIPAddress) (*virtv2.VirtualMachineIPAddressLease, error) {
	// 1. Trying to find the Lease in the local cache.
	{
		leases := &virtv2.VirtualMachineIPAddressLeaseList{}
		err := s.client.List(ctx, leases, &client.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{annotations.LabelVirtualMachineIPAddressUID: string(vmip.GetUID())}),
		})
		if err != nil {
			return nil, fmt.Errorf("list leases in local cache: %w", err)
		}

		switch {
		case len(leases.Items) == 0:
			// Not found.
		case len(leases.Items) == 1:
			return &leases.Items[0], nil
		default:
			return nil, fmt.Errorf("more than one (%d) VirtualMachineIPAddressLease found in the local cache", len(leases.Items))
		}
	}

	// The local cache might be outdated, which is why the Lease is not present in the cache, even though it may already exist in the cluster.
	// Double-check Lease existence in the cluster by making a direct request to the Kubernetes API.
	{
		leases, err := s.virtClient.VirtualMachineIPAddressLeases().List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", annotations.LabelVirtualMachineIPAddressUID, string(vmip.GetUID())),
		})
		if err != nil {
			return nil, fmt.Errorf("list leases via direct request to kubeapi: %w", err)
		}

		switch {
		case len(leases.Items) == 0:
			return nil, nil
		case len(leases.Items) == 1:
			logger.FromContext(ctx).Warn("The lease was not found by label in the local cache, but it already exists in the cluster", "leaseName", leases.Items[0].Name)
			return &leases.Items[0], nil
		default:
			return nil, fmt.Errorf("more than one (%d) VirtualMachineIPAddressLease found via a direct request to kubeapi", len(leases.Items))
		}
	}
}

func toIPNet(prefix netip.Prefix) *net.IPNet {
	return &net.IPNet{
		IP:   prefix.Masked().Addr().AsSlice(),
		Mask: net.CIDRMask(prefix.Bits(), prefix.Addr().BitLen()),
	}
}

func isFirstLastIP(ip netip.Addr, cidr netip.Prefix) (bool, error) {
	ipNet := toIPNet(cidr)
	size := int(k8snet.RangeSize(ipNet))

	first, err := k8snet.GetIndexedIP(ipNet, 0)
	if err != nil {
		return false, err
	}

	if first.Equal(ip.AsSlice()) {
		return true, nil
	}

	last, err := k8snet.GetIndexedIP(ipNet, size-1)
	if err != nil {
		return false, err
	}

	return last.Equal(ip.AsSlice()), nil
}

func getAssignedIPAddress(vmip *virtv2.VirtualMachineIPAddress) string {
	if vmip.Spec.StaticIP != "" {
		return vmip.Spec.StaticIP
	}

	if vmip.Status.Address != "" {
		return vmip.Status.Address
	}

	return ""
}
