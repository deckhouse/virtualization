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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
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

// IPAddressLeaseGVR is the GroupVersionResource for the cluster-scoped SDN
// IPAddressLease resource.
var IPAddressLeaseGVR = schema.GroupVersionResource{
	Group:    "network.deckhouse.io",
	Version:  "v1alpha1",
	Resource: "ipaddressleases",
}

// releaseOrphanedSDNLeases deletes orphaned IPAddressLease objects holding the
// given static IP. The SDN keeps a lease alive for spec.ttl (1h in the e2e
// pool) after its IPAddress is deleted so the owner can reclaim the address;
// an orphaned lease left by a previous e2e run therefore blocks a new Static
// request for the same IP until the TTL expires. Orphaned leases
// (status.orphaningTimestamp set) have no live owner, so deleting them is safe.
// TODO: drop this workaround if the SDN learns to hand an orphaned lease over
// to a new Static IPAddress requesting the same IP.
func releaseOrphanedSDNLeases(ctx context.Context, staticIP string) error {
	leases, err := framework.GetClients().DynamicClient().Resource(IPAddressLeaseGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list SDN IPAddressLeases: %w", err)
	}
	for _, lease := range leases.Items {
		ip, _, _ := unstructured.NestedString(lease.Object, "spec", "ip")
		if ip != staticIP {
			continue
		}
		orphanedAt, _, _ := unstructured.NestedString(lease.Object, "status", "orphaningTimestamp")
		if orphanedAt == "" {
			continue
		}
		err = framework.GetClients().DynamicClient().Resource(IPAddressLeaseGVR).Delete(ctx, lease.GetName(), metav1.DeleteOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("delete orphaned SDN IPAddressLease %s (%s): %w", lease.GetName(), staticIP, err)
		}
	}
	return nil
}

// CreateSDNIPAddress creates an SDN IPAddress resource (type Static) in the given
// namespace, referencing the given network with the specified static IP.
// Uses the dynamic client.
func CreateSDNIPAddress(ctx context.Context, f *framework.Framework, name, namespace, networkKind, networkName, staticIP string) error {
	if err := releaseOrphanedSDNLeases(ctx, staticIP); err != nil {
		return err
	}

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

// GetSDNAllocatedAddress returns the allocated address from the SDN IPAddress
// status. Returns empty string if the resource does not exist or the address
// is not yet allocated.
func GetSDNAllocatedAddress(ctx context.Context, f *framework.Framework, name, namespace string) (string, error) {
	obj, err := framework.GetClients().DynamicClient().Resource(IPAddressGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("get SDN IPAddress %s: %w", name, err)
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

// UntilSDNIPAddressAllocated waits for the SDN IPAddress to report the
// expected allocated address, observing the CR through a dynamic watch.
func UntilSDNIPAddressAllocated(ctx context.Context, name, namespace, expectedAddress string, timeout time.Duration) {
	GinkgoHelper()

	obs, err := observer.New[*unstructured.Unstructured](
		ctx,
		observer.DynamicWatcher(framework.GetClients().DynamicClient(), IPAddressGVR, namespace),
		name, namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for SDN IPAddress %s/%s", namespace, name)
	defer obs.Stop()

	err = obs.WaitFor(sdnAddressAllocated(expectedAddress), timeout)
	Expect(err).NotTo(HaveOccurred(), "SDN IPAddress %s/%s should allocate address %s", namespace, name, expectedAddress)
}

// sdnAddressAllocated reports the SDN IPAddress CR carries the expected
// allocated address in its status.
func sdnAddressAllocated(expectedAddress string) observer.Predicate[*unstructured.Unstructured] {
	return func(u *unstructured.Unstructured) (bool, error) {
		address, _, _ := unstructured.NestedString(u.Object, "status", "address")
		return address == expectedAddress, nil
	}
}
