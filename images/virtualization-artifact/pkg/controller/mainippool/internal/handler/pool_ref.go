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
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const poolRefHandlerName = "PoolRefHandler"

// FieldOwner is the field manager of the server-side apply requests of this
// controller: the pool ranges and the ipam reference of the main network belong
// to this module and to nobody else.
const FieldOwner = "virtualization-controller"

// PoolRefHandler binds the pool to the PrimaryCNI ClusterNetwork "main" through
// spec.ipam.ipAddressPoolRef. With that reference the sdn module allocates the
// addresses of the Main network from the pool.
type PoolRefHandler struct {
	client client.Client
}

func NewPoolRefHandler(client client.Client) *PoolRefHandler {
	return &PoolRefHandler{client: client}
}

func (h *PoolRefHandler) Name() string {
	return poolRefHandlerName
}

func (h *PoolRefHandler) Handle(ctx context.Context) error {
	clusterNetwork := &sdnapi.ClusterNetwork{}
	if err := h.client.Get(ctx, types.NamespacedName{Name: network.MainClusterNetworkName}, clusterNetwork); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if clusterNetwork.Spec.Type != sdnapi.ClusterNetworkTypePrimaryCNI {
		return nil
	}
	var kind, name string
	if ipam := clusterNetwork.Spec.IPAM; ipam != nil && ipam.IPAddressPoolRef != nil {
		kind, name = ipam.IPAddressPoolRef.Kind, ipam.IPAddressPoolRef.Name
	}
	if kind == sdnapi.ClusterIPAddressPoolKind && name == network.MainIPAddressPoolName {
		return nil
	}
	// A pool somebody else put there is their decision, not a drift to correct.
	// The sdn module serves addresses from it just as well; ours is left unused.
	if name != "" {
		logger.FromContext(ctx).Warn("The main ClusterNetwork references another address pool, leaving it as it is",
			"pool", name, "kind", kind, "ownPool", network.MainIPAddressPoolName)
		return nil
	}

	// Unstructured rather than sdnapi.ClusterNetwork: the typed object would apply an
	// empty spec.type along with the reference.
	ref := &unstructured.Unstructured{}
	ref.SetGroupVersionKind(network.ClusterNetworkGVK)
	ref.SetName(network.MainClusterNetworkName)
	if err := unstructured.SetNestedField(ref.Object, map[string]any{
		"kind": sdnapi.ClusterIPAddressPoolKind,
		"name": network.MainIPAddressPoolName,
	}, "spec", "ipam", "ipAddressPoolRef"); err != nil {
		return err
	}
	// Apply carries this one field, so the rest of the network — the business of the
	// sdn module — is not touched from here.
	if err := h.client.Patch(ctx, ref, client.Apply, client.FieldOwner(FieldOwner), client.ForceOwnership); err != nil {
		return fmt.Errorf("apply the ipam reference of ClusterNetwork %s: %w", network.MainClusterNetworkName, err)
	}
	return nil
}
