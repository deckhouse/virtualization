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

package service

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

// FreezeService freezes and unfreezes the guest filesystem. A call carries the UID of the
// VirtualMachineAccessClaim it is made for, which the apiserver checks against the access lock; an empty
// UID makes a call outside any claim, which proceeds only while nothing holds the virtual machine.
type FreezeService struct {
	virtClient kubeclient.Client
}

func NewFreezeService(virtClient kubeclient.Client) *FreezeService {
	return &FreezeService{virtClient: virtClient}
}

// Freeze asks the guest to freeze its filesystem. unfreezeTimeout arms the safety unfreeze in the guest,
// which unfreezes it even if the coordinator never asks.
func (s *FreezeService) Freeze(ctx context.Context, namespace, name string, claimUID types.UID, unfreezeTimeout time.Duration) error {
	err := s.virtClient.VirtualMachines(namespace).Freeze(ctx, name, subv1alpha2.VirtualMachineFreeze{
		UnfreezeTimeout: &metav1.Duration{Duration: unfreezeTimeout},
		ClaimUID:        string(claimUID),
	})
	if err != nil {
		return fmt.Errorf("freeze the filesystem of the virtual machine %s/%s: %w", namespace, name, err)
	}

	return nil
}

// Unfreeze asks the guest to unfreeze its filesystem.
func (s *FreezeService) Unfreeze(ctx context.Context, namespace, name string, claimUID types.UID) error {
	err := s.virtClient.VirtualMachines(namespace).Unfreeze(ctx, name, subv1alpha2.VirtualMachineUnfreeze{
		ClaimUID: string(claimUID),
	})
	if err != nil {
		return fmt.Errorf("unfreeze the filesystem of the virtual machine %s/%s: %w", namespace, name, err)
	}

	return nil
}
