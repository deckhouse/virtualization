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

package precheck

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dv1alpha1 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha1"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	sdnModuleName         = "sdn"
	sdnModuleCheckEnvName = "SDN_MODULE_PRECHECK"

	// Required VLAN IDs for e2e tests.
	// WithIPPoolNetworkVLANID is a ClusterNetwork with an IPAM pool bound
	// (e2e-ipam-pool, 192.168.200.0/24), used by IPAM tests.
	WithIPPoolNetworkVLANID = 4006
	// L2OnlyNetworkVLANID is a ClusterNetwork without an IPAM pool, used by
	// additional-network tests that configure IPs manually inside the guest OS.
	L2OnlyNetworkVLANID = 4007

	// e2eIPAMPoolName is the name of the ClusterIPAddressPool used for IPAM e2e tests.
	e2eIPAMPoolName = "e2e-ipam-pool"
)

// ClusterIPAddressPoolCreateCommand returns the kubectl command to create the
// ClusterIPAddressPool required for IPAM e2e tests.
const clusterIPAddressPoolCreateCommand = `kubectl apply -f - <<EOF
apiVersion: network.deckhouse.io/v1alpha1
kind: ClusterIPAddressPool
metadata:
  name: %s
spec:
  leaseTTL: 1h
  pools:
    - network: 192.168.200.0/24
EOF`

// clusterNetworkCreateTemplate is the base kubectl apply command for a
// ClusterNetwork. The trailing %s is an optional spec stanza (e.g. the ipam
// block) appended by ClusterNetworkCreateCommandWithIPAM.
const clusterNetworkCreateTemplate = `kubectl apply -f - <<EOF
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
    id: %d%s
EOF`

// clusterNetworkIPAMBlock is the optional spec.ipam stanza appended to
// clusterNetworkCreateTemplate when an IPAM pool must be bound to the network.
const clusterNetworkIPAMBlock = `
  ipam:
    ipAddressPoolRef:
      kind: ClusterIPAddressPool
      name: %s`

// clusterNetworkGVR is the GroupVersionResource for the SDN ClusterNetwork resource.
var clusterNetworkGVR = schema.GroupVersionResource{
	Group:    "network.deckhouse.io",
	Version:  "v1alpha1",
	Resource: "clusternetworks",
}

// clusterIPAddressPoolGVR is the GroupVersionResource for the SDN
// ClusterIPAddressPool resource.
var clusterIPAddressPoolGVR = schema.GroupVersionResource{
	Group:    "network.deckhouse.io",
	Version:  "v1alpha1",
	Resource: "clusteripaddresspools",
}

// isClusterIPAddressPoolExists reports whether a ClusterIPAddressPool with
// the given name exists in the cluster.
func isClusterIPAddressPoolExists(ctx context.Context, f *framework.Framework, name string) bool {
	GinkgoHelper()

	_, err := f.DynamicClient().Resource(clusterIPAddressPoolGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		_, _ = fmt.Fprintf(GinkgoWriter, "error checking ClusterIPAddressPool %s: %v\n", name, err)
	}
	return err == nil || !k8serrors.IsNotFound(err)
}

// getClusterNetwork fetches the ClusterNetwork with the given name via the
// dynamic client.
func getClusterNetwork(ctx context.Context, f *framework.Framework, name string) (*unstructured.Unstructured, error) {
	return f.DynamicClient().Resource(clusterNetworkGVR).Get(ctx, name, metav1.GetOptions{})
}

// getClusterNetworkIPAMPoolName returns the name of the ClusterIPAddressPool bound
// to the given ClusterNetwork (spec.ipam.ipAddressPoolRef.name), or an empty
// string if no IPAM pool is configured.
func getClusterNetworkIPAMPoolName(ctx context.Context, f *framework.Framework, name string) (string, error) {
	obj, err := getClusterNetwork(ctx, f, name)
	if err != nil {
		return "", err
	}
	poolName, found, err := unstructured.NestedString(obj.Object, "spec", "ipam", "ipAddressPoolRef", "name")
	if err != nil {
		return "", err
	}
	if !found {
		return "", nil
	}
	return poolName, nil
}

// ClusterNetworkName returns the name of ClusterNetwork for given VLAN ID.
func ClusterNetworkName(vlanID int) string {
	return fmt.Sprintf("cn-%d-for-e2e-test", vlanID)
}

// ClusterNetworkCreateCommand returns the kubectl command to create ClusterNetwork for given VLAN ID.
func ClusterNetworkCreateCommand(vlanID int) string {
	return fmt.Sprintf(clusterNetworkCreateTemplate, ClusterNetworkName(vlanID), vlanID, "")
}

// ClusterNetworkCreateCommandWithIPAM returns the kubectl command to create ClusterNetwork
// for given VLAN ID with an IPAM pool reference bound under spec.
func ClusterNetworkCreateCommandWithIPAM(vlanID int, poolName string) string {
	return fmt.Sprintf(clusterNetworkCreateTemplate, ClusterNetworkName(vlanID), vlanID,
		fmt.Sprintf(clusterNetworkIPAMBlock, poolName))
}

// IsClusterNetworkExists checks if ClusterNetwork with given VLAN ID exists.
func IsClusterNetworkExists(ctx context.Context, f *framework.Framework, vlanID int) bool {
	GinkgoHelper()

	name := ClusterNetworkName(vlanID)
	_, err := getClusterNetwork(ctx, f, name)
	if err != nil && !k8serrors.IsNotFound(err) {
		_, _ = fmt.Fprintf(GinkgoWriter, "error checking ClusterNetwork %s: %v\n", name, err)
	}

	return err == nil || !k8serrors.IsNotFound(err)
}

// requiredClusterNetwork describes a ClusterNetwork that must exist for e2e
// tests and, when poolName is set, must reference that ClusterIPAddressPool.
type requiredClusterNetwork struct {
	vlanID   int
	poolName string // when non-empty, the network must bind this ClusterIPAddressPool
}

func (r requiredClusterNetwork) getName() string { return ClusterNetworkName(r.vlanID) }

// createCommand returns the kubectl command that creates this ClusterNetwork,
// including the ipam stanza when a pool is required.
func (r requiredClusterNetwork) createCommand() string {
	if r.poolName == "" {
		return ClusterNetworkCreateCommand(r.vlanID)
	}
	return ClusterNetworkCreateCommandWithIPAM(r.vlanID, r.poolName)
}

// recreateCommand returns the kubectl command that deletes and recreates
// this ClusterNetwork. The spec.ipam stanza is immutable, so when the bound
// pool must change the existing network has to be removed first.
func (r requiredClusterNetwork) recreateCommand() string {
	return fmt.Sprintf("kubectl delete clusternetwork.network.deckhouse.io %s && %s", r.getName(), r.createCommand())
}

// verify ensures the required ClusterNetwork exists and, when a pool is
// required, that the ClusterIPAddressPool exists and is bound to the network.
// The returned error embeds the exact kubectl command needed to remediate the
// failure.
func (r requiredClusterNetwork) verify(ctx context.Context, f *framework.Framework) error {
	if r.poolName != "" && !isClusterIPAddressPoolExists(ctx, f, r.poolName) {
		return fmt.Errorf("%s=no to disable this precheck: ClusterIPAddressPool %q does not exist. Create it first: %s",
			sdnModuleCheckEnvName, r.poolName, fmt.Sprintf(clusterIPAddressPoolCreateCommand, r.poolName))
	}
	if !IsClusterNetworkExists(ctx, f, r.vlanID) {
		return fmt.Errorf("%s=no to disable this precheck: ClusterNetwork %q does not exist. Create it first: %s",
			sdnModuleCheckEnvName, r.getName(), r.createCommand())
	}
	if r.poolName == "" {
		return nil
	}
	poolName, err := getClusterNetworkIPAMPoolName(ctx, f, r.getName())
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to check IPAM pool on ClusterNetwork %q: %w",
			sdnModuleCheckEnvName, r.getName(), err)
	}
	if poolName == "" {
		return fmt.Errorf("%s=no to disable this precheck: ClusterNetwork %q has no IPAM pool configured. spec.ipam is immutable, delete and recreate the network with the expected pool: %s",
			sdnModuleCheckEnvName, r.getName(), r.recreateCommand())
	}
	if poolName != r.poolName {
		return fmt.Errorf("%s=no to disable this precheck: ClusterNetwork %q is bound to IPAM pool %q, expected %q. spec.ipam is immutable, delete and recreate the network with the expected pool: %s",
			sdnModuleCheckEnvName, r.getName(), poolName, r.poolName, r.recreateCommand())
	}
	return nil
}

// sdnPrecheck implements Precheck interface for SDN module.
type sdnPrecheck struct{}

func (s *sdnPrecheck) Label() string {
	return PrecheckSDN
}

func (s *sdnPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(sdnModuleCheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("SDN module check is disabled.\n"))
		return nil
	}

	if !IsModuleEnabled(ctx, f, sdnModuleName) {
		return fmt.Errorf("%s=no to disable this precheck: SDN module should be enabled", sdnModuleCheckEnvName)
	}

	sdnModule := &dv1alpha1.Module{}
	err := f.GenericClient().Get(ctx, client.ObjectKey{Name: sdnModuleName}, sdnModule)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to check SDN module status: %w", sdnModuleCheckEnvName, err)
	}
	if sdnModule.Status.Phase != modulePhaseReady {
		return fmt.Errorf("%s=no to disable this precheck: SDN module should be ready; current status: %s", sdnModuleCheckEnvName, sdnModule.Status.Phase)
	}

	// Check required ClusterNetworks for e2e tests.
	for _, r := range []requiredClusterNetwork{
		{vlanID: WithIPPoolNetworkVLANID, poolName: e2eIPAMPoolName},
		{vlanID: L2OnlyNetworkVLANID},
	} {
		if err := r.verify(ctx, f); err != nil {
			return err
		}
	}

	return nil
}

// Register SDN precheck (not common - requires explicit label).
func init() {
	RegisterPrecheck(&sdnPrecheck{}, false)
}
