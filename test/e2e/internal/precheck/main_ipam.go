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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
	dv1alpha1 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha1"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const mainIPAMCheckEnvName = "MAIN_IPAM_PRECHECK"

type mainIPAMPrecheck struct{}

func (p *mainIPAMPrecheck) Label() string {
	return PrecheckMainIPAM
}

func (p *mainIPAMPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(mainIPAMCheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("Main IPAM check is disabled.\n"))
		return nil
	}

	if !IsModuleEnabledByConfig(ctx, f, sdnModuleName) {
		return fmt.Errorf("%s=no to disable this precheck: the sdn module should be enabled", mainIPAMCheckEnvName)
	}

	sdnModule := &dv1alpha1.Module{}
	err := f.GenericClient().Get(ctx, client.ObjectKey{Name: sdnModuleName}, sdnModule)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to check the sdn module status: %w", mainIPAMCheckEnvName, err)
	}
	if sdnModule.Status.Phase != modulePhaseReady {
		return fmt.Errorf("%s=no to disable this precheck: the sdn module should be ready; current status: %s", mainIPAMCheckEnvName, sdnModule.Status.Phase)
	}

	clusterNetwork := &sdnapi.ClusterNetwork{}
	err = f.GenericClient().Get(ctx, client.ObjectKey{Name: network.MainClusterNetworkName}, clusterNetwork)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return fmt.Errorf("%s=no to disable this precheck: ClusterNetwork %q does not exist", mainIPAMCheckEnvName, network.MainClusterNetworkName)
		}
		return fmt.Errorf("%s=no to disable this precheck: failed to get ClusterNetwork %q: %w", mainIPAMCheckEnvName, network.MainClusterNetworkName, err)
	}

	if clusterNetwork.Spec.Type != sdnapi.ClusterNetworkTypePrimaryCNI {
		return fmt.Errorf("%s=no to disable this precheck: ClusterNetwork %q has spec.type %q, expected %q",
			mainIPAMCheckEnvName, network.MainClusterNetworkName, clusterNetwork.Spec.Type, sdnapi.ClusterNetworkTypePrimaryCNI)
	}

	var poolName string
	if clusterNetwork.Spec.IPAM != nil && clusterNetwork.Spec.IPAM.IPAddressPoolRef != nil {
		poolName = clusterNetwork.Spec.IPAM.IPAddressPoolRef.Name
	}
	if poolName == "" {
		return fmt.Errorf("%s=no to disable this precheck: ClusterNetwork %q has no IPAM pool configured (spec.ipam.ipAddressPoolRef)",
			mainIPAMCheckEnvName, network.MainClusterNetworkName)
	}
	if !isClusterIPAddressPoolExists(ctx, f, poolName) {
		return fmt.Errorf("%s=no to disable this precheck: ClusterIPAddressPool %q referenced by ClusterNetwork %q does not exist",
			mainIPAMCheckEnvName, poolName, network.MainClusterNetworkName)
	}

	return nil
}

// Register Main IPAM precheck (not common - requires explicit label).
func init() {
	RegisterPrecheck(&mainIPAMPrecheck{}, false)
}
