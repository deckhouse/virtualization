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
	"math/rand"
	"regexp"
	"strings"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/mac"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const MaxCount int = 16777216

type MACAddressService struct {
	oui        string
	client     client.Client
	virtClient kubeclient.Client
}

func NewMACAddressService(
	clusterUUID string,
	client client.Client,
	virtClient kubeclient.Client,
) *MACAddressService {
	oui := generateOUI(clusterUUID)

	oui, err := formatOUI(oui)
	if err != nil {
		return nil
	}

	return &MACAddressService{
		oui:        oui,
		client:     client,
		virtClient: virtClient,
	}
}

func generateOUI(clusterUID string) string {
	if !validateUUID(clusterUID) {
		return ""
	}

	cleanUID := strings.ReplaceAll(clusterUID, "-", "")
	numBytes := len(cleanUID) / 2
	for i := 0; i < numBytes; i++ {
		switch cleanUID[2*i+1] {
		case '6', '2', 'a', 'e':
			start := 2 * i
			var oui string
			if start+6 <= len(cleanUID) {
				oui = cleanUID[start : start+6]
			} else {
				oui = cleanUID[start:]
				oui += cleanUID[:(6 - len(oui))]
			}
			return oui
		}
	}

	oui := cleanUID[:6]
	oui = oui[:1] + "2" + oui[2:]

	return oui
}

func validateUUID(uid string) bool {
	matched, _ := regexp.MatchString("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$", uid)
	return matched
}

func (s MACAddressService) IsAvailableAddress(address string, allocatedMACs mac.AllocatedMACs) error {
	if !mac.IsValidAddressFormat(address) {
		return errors.New("invalid MAC address format")
	}

	if _, ok := allocatedMACs[address]; ok {
		// already exists
		return ErrMACAddressAlreadyExist
	}

	return nil
}

func formatOUI(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)

	re := regexp.MustCompile(`(?i)([0-9A-Fa-f]{2})`)
	matches := re.FindAllString(prefix, -1)

	if len(matches) != 3 {
		return "", fmt.Errorf("wrong format MAC address oui")
	}

	return fmt.Sprintf("%s:%s:%s", matches[0], matches[1], matches[2]), nil
}

func (s MACAddressService) AllocateNewAddress(allocatedMACs mac.AllocatedMACs) (string, error) {
	prefix, err := formatOUI(s.oui)
	if err != nil {
		return "", err
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	retry := 0
	maxRetries := MaxCount - len(allocatedMACs)

	for retry < maxRetries {
		genAddress := fmt.Sprintf("%s:%02x:%02x:%02x", prefix, r.Intn(256), r.Intn(256), r.Intn(256))
		if _, ok := allocatedMACs[genAddress]; !ok {
			return genAddress, nil
		}
		retry++
	}

	return "", errors.New("no remaining MAC addresses")
}

func (s MACAddressService) GetAllocatedAddresses(ctx context.Context) (mac.AllocatedMACs, error) {
	var leases virtv2.VirtualMachineMACAddressLeaseList

	err := s.client.List(ctx, &leases)
	if err != nil {
		return nil, fmt.Errorf("error getting leases: %w", err)
	}

	allocatedMACs := make(mac.AllocatedMACs, len(leases.Items))
	for _, lease := range leases.Items {
		allocatedMACs[mac.LeaseNameToAddress(lease.Name)] = &lease
	}

	return allocatedMACs, nil
}

func (s MACAddressService) GetLease(ctx context.Context, vmmac *virtv2.VirtualMachineMACAddress) (*virtv2.VirtualMachineMACAddressLease, error) {
	// The MAC address cannot be changed for a vmmac. Once it has been assigned, it will remain the same.
	macAddress := getAssignedMACAddress(vmmac)
	if macAddress != "" {
		return s.getLeaseByMACAddress(ctx, macAddress)
	}

	// Either the Lease hasn't been created yet, or the address hasn't been set yet.
	// We need to make sure the Lease doesn't exist in the cluster by searching for it by label.
	return s.getLeaseByLabel(ctx, vmmac)
}

func (s MACAddressService) getLeaseByMACAddress(ctx context.Context, macAddress string) (*virtv2.VirtualMachineMACAddressLease, error) {
	// 1. Trying to find the Lease in the local cache.
	lease, err := object.FetchObject(ctx, types.NamespacedName{Name: mac.AddressToLeaseName(macAddress)}, s.client, &virtv2.VirtualMachineMACAddressLease{})
	if err != nil {
		return nil, fmt.Errorf("fetch lease in local cache: %w", err)
	}

	if lease != nil {
		return lease, nil
	}

	// The local cache might be outdated, which is why the Lease is not present in the cache, even though it may already exist in the cluster.
	// Double-check Lease existence in the cluster by making a direct request to the Kubernetes API.
	lease, err = s.virtClient.VirtualMachineMACAddressLeases().Get(ctx, mac.AddressToLeaseName(macAddress), metav1.GetOptions{})
	switch {
	case err == nil:
		logger.FromContext(ctx).Warn("The lease was not found by mac address in the local cache, but it already exists in the cluster", "leaseName", lease.Name)
		return lease, nil
	case k8serrors.IsNotFound(err):
		return nil, nil
	default:
		return nil, fmt.Errorf("get lease via direct request to kubeapi: %w", err)
	}
}

func (s MACAddressService) getLeaseByLabel(ctx context.Context, vmmac *virtv2.VirtualMachineMACAddress) (*virtv2.VirtualMachineMACAddressLease, error) {
	// 1. Trying to find the Lease in the local cache.
	{
		leases := &virtv2.VirtualMachineMACAddressLeaseList{}
		err := s.client.List(ctx, leases, &client.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{annotations.LabelVirtualMachineMACAddressUID: string(vmmac.GetUID())}),
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
			return nil, fmt.Errorf("more than one (%d) VirtualMachineMACAddressLease found in the local cache", len(leases.Items))
		}
	}

	// The local cache might be outdated, which is why the Lease is not present in the cache, even though it may already exist in the cluster.
	// Double-check Lease existence in the cluster by making a direct request to the Kubernetes API.
	{
		leases, err := s.virtClient.VirtualMachineMACAddressLeases().List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", annotations.LabelVirtualMachineMACAddressUID, string(vmmac.GetUID())),
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
			return nil, fmt.Errorf("more than one (%d) VirtualMachineMACAddressLease found via a direct request to kubeapi", len(leases.Items))
		}
	}
}

func getAssignedMACAddress(vmmac *virtv2.VirtualMachineMACAddress) string {
	if vmmac.Spec.Address != "" {
		return vmmac.Spec.Address
	}

	if vmmac.Status.Address != "" {
		return vmmac.Status.Address
	}

	return ""
}
