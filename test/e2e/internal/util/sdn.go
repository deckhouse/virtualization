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

package util

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

func ClusterNetworkName(vlanID int) string {
	return fmt.Sprintf("cn-%d-for-e2e-test", vlanID)
}

func ClusterNetworkCreateCommand(vlanID int) string {
	return fmt.Sprintf(`kubectl apply -f - <<EOF
apiVersion: network.deckhouse.io/v1alpha1
kind: ClusterNetwork
metadata:
  name: %s
spec:
  parentNodeNetworkInterfaces:
    labelSelector:
      matchLabels:
        network.deckhouse.io/interface-type: NIC
        network.deckhouse.io/node-role: worker
  type: VLAN
  vlan:
    id: %d
EOF`, ClusterNetworkName(vlanID), vlanID)
}

func IsSdnModuleEnabled(f *framework.Framework) bool {
	GinkgoHelper()

	sdnModule, err := f.GetModuleConfig(context.Background(), "sdn")
	Expect(err).NotTo(HaveOccurred())
	enabled := sdnModule.Spec.Enabled

	return enabled != nil && *enabled
}

func IsClusterNetworkExists(f *framework.Framework, vlanID int) bool {
	GinkgoHelper()

	gvr := schema.GroupVersionResource{
		Group:    "network.deckhouse.io",
		Version:  "v1alpha1",
		Resource: "clusternetworks",
	}

	_, err := framework.GetClients().DynamicClient().Resource(gvr).Get(context.Background(), ClusterNetworkName(vlanID), metav1.GetOptions{})
	Expect(err).To(SatisfyAny(BeNil(), WithTransform(k8serrors.IsNotFound, BeTrue())))

	return err == nil || !k8serrors.IsNotFound(err)
}

// IPAddressGVR is the GroupVersionResource for the SDN IPAddress resource.
var IPAddressGVR = schema.GroupVersionResource{
	Group:    "network.deckhouse.io",
	Version:  "v1alpha1",
	Resource: "ipaddresses",
}

// CreateSDNIPAddress creates an SDN IPAddress resource (type Static) in the given
// namespace, referencing the given network with the specified static IP.
// Uses the dynamic client.
func CreateSDNIPAddress(ctx context.Context, f *framework.Framework, name, namespace, networkKind, networkName, staticIP string) error {
	ipAddr := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "network.deckhouse.io/v1alpha1",
		"kind":       "IPAddress",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"networkRef": map[string]any{
				"kind": networkKind,
				"name": networkName,
			},
			"type": "Static",
			"static": map[string]any{
				"ip": staticIP,
			},
		},
	}}
	_, err := framework.GetClients().DynamicClient().Resource(IPAddressGVR).Namespace(namespace).Create(ctx, ipAddr, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create SDN IPAddress %s: %w", name, err)
	}
	return nil
}

// GetSDNIPAddress returns the SDN IPAddress resource by name in the given namespace.
// Returns nil (without error) if not found.
func GetSDNIPAddress(ctx context.Context, f *framework.Framework, name, namespace string) (*unstructured.Unstructured, error) {
	obj, err := framework.GetClients().DynamicClient().Resource(IPAddressGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get SDN IPAddress %s: %w", name, err)
	}
	return obj, nil
}

// GetSDNIPAddressAddress returns the allocated address from the SDN IPAddress status.
// Returns empty string if the address is not yet allocated.
func GetSDNIPAddressAddress(ctx context.Context, f *framework.Framework, name, namespace string) (string, error) {
	obj, err := GetSDNIPAddress(ctx, f, name, namespace)
	if err != nil || obj == nil {
		return "", err
	}
	addr, _, _ := unstructured.NestedString(obj.Object, "status", "address")
	return addr, nil
}

// DeleteSDNIPAddress deletes the SDN IPAddress by name in the given namespace.
func DeleteSDNIPAddress(ctx context.Context, f *framework.Framework, name, namespace string) error {
	err := framework.GetClients().DynamicClient().Resource(IPAddressGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete SDN IPAddress %s: %w", name, err)
	}
	return nil
}

// ListSDNIPAddresses lists all SDN IPAddress resources in the given namespace.
func ListSDNIPAddresses(ctx context.Context, f *framework.Framework, namespace string) (*unstructured.UnstructuredList, error) {
	list, err := framework.GetClients().DynamicClient().Resource(IPAddressGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list SDN IPAddress in %s: %w", namespace, err)
	}
	return list, nil
}

// DeleteAllSDNIPAddresses deletes all SDN IPAddress resources in the given namespace.
// Used for cleanup after tests.
func DeleteAllSDNIPAddresses(ctx context.Context, f *framework.Framework, namespace string) error {
	return framework.GetClients().DynamicClient().Resource(IPAddressGVR).Namespace(namespace).
		DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
}
