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

package network

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var (
	NetworkGVK        = schema.GroupVersionKind{Group: "network.deckhouse.io", Version: "v1alpha1", Kind: "Network"}
	ClusterNetworkGVK = schema.GroupVersionKind{Group: "network.deckhouse.io", Version: "v1alpha1", Kind: "ClusterNetwork"}
)

func SpecKey(netSpec v1alpha2.NetworksSpec) string {
	return fmt.Sprintf("%s/%s", netSpec.Type, netSpec.Name)
}

func IsNetworkSpecReady(ctx context.Context, c client.Client, namespace string, netSpec v1alpha2.NetworksSpec) (bool, error) {
	if netSpec.Type == v1alpha2.NetworksTypeMain {
		return true, nil
	}
	obj, found, err := getNetworkObject(ctx, c, namespace, netSpec)
	if err != nil || !found {
		return false, err
	}
	return isReadyTrue(obj)
}

func isReadyTrue(obj *unstructured.Unstructured) (bool, error) {
	conds, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil {
		return false, fmt.Errorf("read status.conditions of %s/%s: %w", obj.GetKind(), obj.GetName(), err)
	}
	if !found {
		return false, nil
	}
	for _, c := range conds {
		condMap, ok := c.(map[string]any)
		if !ok {
			continue
		}
		typ, _, err := unstructured.NestedString(condMap, "type")
		if err != nil {
			return false, fmt.Errorf("read condition.type of %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}
		if typ != "Ready" {
			continue
		}
		status, _, err := unstructured.NestedString(condMap, "status")
		if err != nil {
			return false, fmt.Errorf("read Ready condition.status of %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}
		return status == string(metav1.ConditionTrue), nil
	}
	return false, nil
}

// HasIPAM reports whether the referenced additional network has IPAM configured,
// i.e. a pool bound to it via spec.ipam.ipAddressPoolRef.
// Returns false for the Main network (no IPAM via SDN).
func HasIPAM(ctx context.Context, c client.Client, namespace string, netSpec v1alpha2.NetworksSpec) (bool, error) {
	if netSpec.Type == v1alpha2.NetworksTypeMain {
		return false, nil
	}
	obj, found, err := getNetworkObject(ctx, c, namespace, netSpec)
	if err != nil || !found {
		return false, err
	}
	return hasIPAddressPoolRef(obj)
}

// getNetworkObject fetches the referenced additional Network/ClusterNetwork as an
// unstructured object. It returns (nil, false, nil) for unknown network types or
// when the network does not exist. The Main network is handled by the caller.
func getNetworkObject(ctx context.Context, c client.Client, namespace string, netSpec v1alpha2.NetworksSpec) (*unstructured.Unstructured, bool, error) {
	obj := &unstructured.Unstructured{}
	key := types.NamespacedName{Name: netSpec.Name}
	switch netSpec.Type {
	case v1alpha2.NetworksTypeClusterNetwork:
		obj.SetGroupVersionKind(ClusterNetworkGVK)
	case v1alpha2.NetworksTypeNetwork:
		obj.SetGroupVersionKind(NetworkGVK)
		key.Namespace = namespace
	default:
		return nil, false, nil
	}
	if err := c.Get(ctx, key, obj); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("get %s %s: %w", obj.GetKind(), netSpec.Name, err)
	}
	return obj, true, nil
}

// hasIPAddressPoolRef reports whether the network object has spec.ipam.ipAddressPoolRef set.
// The pool is considered configured if the ipAddressPoolRef object is present and has a name.
func hasIPAddressPoolRef(obj *unstructured.Unstructured) (bool, error) {
	poolRef, found, err := unstructured.NestedMap(obj.Object, "spec", "ipam", "ipAddressPoolRef")
	if err != nil {
		return false, fmt.Errorf("read spec.ipam.ipAddressPoolRef of %s/%s: %w", obj.GetKind(), obj.GetName(), err)
	}
	if !found {
		return false, nil
	}
	name, _, err := unstructured.NestedString(poolRef, "name")
	if err != nil {
		return false, fmt.Errorf("read spec.ipam.ipAddressPoolRef.name of %s/%s: %w", obj.GetKind(), obj.GetName(), err)
	}
	return name != "", nil
}
