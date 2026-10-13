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

package handler

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
)

const poolHandlerName = "PoolHandler"

// ErrPoolTerminating means the pool is being deleted. Its reference must not be put
// back meanwhile: the sdn module keeps a referenced pool from going away.
var ErrPoolTerminating = errors.New("the address pool is being deleted")

// PoolHandler keeps the ranges of the d8-virtualization ClusterIPAddressPool in
// step with the virtualMachineCIDRs module setting.
type PoolHandler struct {
	client client.Client
	cidrs  []string
}

func NewPoolHandler(client client.Client, cidrs []string) *PoolHandler {
	return &PoolHandler{client: client, cidrs: cidrs}
}

func (h *PoolHandler) Name() string {
	return poolHandlerName
}

// Handle applies the ranges of the pool. Server-side apply names the module as the
// owner of the fields it sets and leaves the rest of the object alone.
func (h *PoolHandler) Handle(ctx context.Context) error {
	pool := &sdnapi.ClusterIPAddressPool{
		TypeMeta:   metav1.TypeMeta{APIVersion: sdnapi.SchemeGroupVersion.String(), Kind: sdnapi.ClusterIPAddressPoolKind},
		ObjectMeta: metav1.ObjectMeta{Name: network.MainIPAddressPoolName},
	}
	for _, cidr := range h.cidrs {
		pool.Spec.Pools = append(pool.Spec.Pools, sdnapi.IPAddressPoolRange{Network: cidr})
	}
	if err := h.client.Patch(ctx, pool, client.Apply, client.FieldOwner(FieldOwner), client.ForceOwnership); err != nil {
		return fmt.Errorf("apply ClusterIPAddressPool %s: %w", network.MainIPAddressPoolName, err)
	}
	if pool.GetDeletionTimestamp() != nil {
		return ErrPoolTerminating
	}
	return nil
}
