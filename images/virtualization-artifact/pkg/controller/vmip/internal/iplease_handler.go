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

package internal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8snet "k8s.io/utils/net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type IPLeaseHandler struct {
	client      client.Client
	logger      logr.Logger
	ParsedCIDRs []*net.IPNet
}

func NewIPLeaseHandler(client client.Client, logger logr.Logger, virtualMachineCIDRs []string) *IPLeaseHandler {
	parsedCIDRs := make([]*net.IPNet, len(virtualMachineCIDRs))

	for i, cidr := range virtualMachineCIDRs {
		_, parsedCIDR, err := net.ParseCIDR(cidr)
		if err != nil || parsedCIDR == nil {
			logger.Error(err, fmt.Sprintf("failed to parse virtual cide %s: %w", cidr, err))
			return nil
		}

		parsedCIDRs[i] = parsedCIDR
	}

	return &IPLeaseHandler{
		client:      client,
		logger:      logger.WithValues("handler", "IPLeaseHandler"),
		ParsedCIDRs: parsedCIDRs,
	}
}

func (h *IPLeaseHandler) Handle(ctx context.Context, state state.VMIPState) (reconcile.Result, error) {
	vmipLease, err := state.VirtualMachineIPLease(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case vmipLease == nil && state.VirtualMachineIP().Current().Spec.VirtualMachineIPAddressLease != "":
		h.logger.Info("Lease by name not found: waiting for the lease to be available")
		return reconcile.Result{}, nil

	case vmipLease == nil:
		// Lease not found by spec.virtualMachineIPAddressLease or spec.Address: it doesn't exist.
		h.logger.Info("No Lease for VirtualMachineIP: create the new one", "address", state.VirtualMachineIP().Current().Spec.Address, "leaseName", state.VirtualMachineIP().Current().Spec.VirtualMachineIPAddressLease)

		leaseName := state.VirtualMachineIP().Current().Spec.VirtualMachineIPAddressLease

		if state.VirtualMachineIP().Current().Spec.Address == "" {
			if leaseName != "" {
				h.logger.Info("VirtualMachineIP address omitted in the spec: extract from the lease name")
				state.VirtualMachineIP().Changed().Spec.Address = util.LeaseNameToIP(leaseName)
			} else {
				h.logger.Info("VirtualMachineIP address omitted in the spec: allocate the new one")
				var err error
				state.VirtualMachineIP().Changed().Spec.Address, err = h.allocateNewIP(state.AllocatedIPs())
				if err != nil {
					return reconcile.Result{}, err
				}
			}
		}

		if !h.isAvailableAddress(state.VirtualMachineIP().Changed().Spec.Address, state.AllocatedIPs()) {
			h.logger.Info("VirtualMachineIP cannot be created: the address has already been allocated for another vmip", "address", state.VirtualMachineIP().Current().Spec.Address)
			return reconcile.Result{}, nil
		}

		if leaseName == "" {
			leaseName = util.IpToLeaseName(state.VirtualMachineIP().Changed().Spec.Address)
		}

		h.logger.Info("Create lease",
			"leaseName", leaseName,
			"reclaimPolicy", state.VirtualMachineIP().Current().Spec.ReclaimPolicy,
			"refName", state.VirtualMachineIP().Name().Name,
			"refNamespace", state.VirtualMachineIP().Name().Namespace,
		)

		state.VirtualMachineIP().Changed().Spec.VirtualMachineIPAddressLease = leaseName

		err := h.client.Update(ctx, state.VirtualMachineIP().Changed())
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to set lease name for vmip: %w", err)
		}

		err = h.client.Create(ctx, &virtv2.VirtualMachineIPAddressLease{
			ObjectMeta: metav1.ObjectMeta{
				Name: leaseName,
			},
			Spec: virtv2.VirtualMachineIPAddressLeaseSpec{
				ReclaimPolicy: state.VirtualMachineIP().Current().Spec.ReclaimPolicy,
				IpAddressRef: &virtv2.VirtualMachineIPAddressLeaseIpAddressRef{
					Name:      state.VirtualMachineIP().Name().Name,
					Namespace: state.VirtualMachineIP().Name().Namespace,
				},
			},
		})
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil

	case vmipLease.Status.Phase == "":
		h.logger.Info("Lease is not ready: waiting for the lease")
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, nil

	case util.IsBoundLease(vmipLease, state.VirtualMachineIP()):
		h.logger.Info("Lease already exists, VirtualMachineIP ref is valid")
		return reconcile.Result{}, nil

	case vmipLease.Status.Phase == virtv2.VirtualMachineIPAddressLeasePhaseBound:
		h.logger.Info("Lease is bounded to another VirtualMachineIP: recreate VirtualMachineIP when the lease is released")
		return reconcile.Result{}, nil

	default:
		h.logger.Info("Lease is released: set binding")

		vmipLease.Spec.ReclaimPolicy = state.VirtualMachineIP().Current().Spec.ReclaimPolicy
		vmipLease.Spec.IpAddressRef = &virtv2.VirtualMachineIPAddressLeaseIpAddressRef{
			Name:      state.VirtualMachineIP().Name().Name,
			Namespace: state.VirtualMachineIP().Name().Namespace,
		}

		err := h.client.Update(ctx, vmipLease)
		if err != nil {
			return reconcile.Result{}, err
		}

		state.VirtualMachineIP().Changed().Spec.VirtualMachineIPAddressLease = vmipLease.Name
		state.VirtualMachineIP().Changed().Spec.Address = util.LeaseNameToIP(vmipLease.Name)

		return reconcile.Result{}, h.client.Update(ctx, state.VirtualMachineIP().Changed())
	}
}

func (h IPLeaseHandler) isAvailableAddress(address string, allocatedIPs util.AllocatedIPs) bool {
	ip := net.ParseIP(address)

	if _, ok := allocatedIPs[ip.String()]; !ok {
		for _, cidr := range h.ParsedCIDRs {
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

func (h IPLeaseHandler) allocateNewIP(allocatedIPs util.AllocatedIPs) (string, error) {
	for _, cidr := range h.ParsedCIDRs {
		for ip := cidr.IP.Mask(cidr.Mask); cidr.Contains(ip); inc(ip) {
			// Allow allocation of IP address explicitly specified using a 32-bit mask.
			if k8snet.RangeSize(cidr) != 1 {
				// Skip the allocation of the first or last addresses within the CIDR range.
				isFirstLast, err := util.IsFirstLastIP(ip, cidr)
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
