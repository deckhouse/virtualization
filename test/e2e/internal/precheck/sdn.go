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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dv1alpha1 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha1"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	sdnModuleName         = "sdn"
	sdnModuleCheckEnvName = "SDN_MODULE_PRECHECK"

	// Required VLAN IDs for e2e tests
	additionalInterfaceVLANID       = 4006
	secondAdditionalInterfaceVLANID = 4007
)

// ClusterNetworkName returns the name of ClusterNetwork for given VLAN ID.
func ClusterNetworkName(vlanID int) string {
	return fmt.Sprintf("cn-%d-for-e2e-test", vlanID)
}

// ClusterNetworkCreateCommand returns the kubectl command to create ClusterNetwork for given VLAN ID.
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

// IsClusterNetworkExists checks if ClusterNetwork with given VLAN ID exists.
func IsClusterNetworkExists(f *framework.Framework, vlanID int) bool {
	GinkgoHelper()

	gvr := schema.GroupVersionResource{
		Group:    "network.deckhouse.io",
		Version:  "v1alpha1",
		Resource: "clusternetworks",
	}

	_, err := f.DynamicClient().Resource(gvr).Get(context.Background(), ClusterNetworkName(vlanID), metav1.GetOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		_, _ = fmt.Fprintf(GinkgoWriter, "error checking ClusterNetwork %s: %v\n", ClusterNetworkName(vlanID), err)
	}

	return err == nil || !k8serrors.IsNotFound(err)
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

	if !IsModuleEnabled(f, sdnModuleName) {
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

	// Check required ClusterNetworks for e2e tests
	for _, vlanID := range []int{additionalInterfaceVLANID, secondAdditionalInterfaceVLANID} {
		if !IsClusterNetworkExists(f, vlanID) {
			return fmt.Errorf("%s=no to disable this precheck: ClusterNetwork %q does not exist. Create it first: %s",
				sdnModuleCheckEnvName, ClusterNetworkName(vlanID), ClusterNetworkCreateCommand(vlanID))
		}
	}

	return nil
}

// Register SDN precheck (not common - requires explicit label).
func init() {
	RegisterPrecheck(&sdnPrecheck{}, false)
}
