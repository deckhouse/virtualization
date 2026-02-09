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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	ClusterNetworkName          = "cn-1003-for-e2e-test"
	ClusterNetworkVLANID        = 1003
	ClusterNetworkCreateCommand = `kubectl apply -f - <<EOF
apiVersion: network.deckhouse.io/v1alpha1
kind: ClusterNetwork
metadata:
  name: cn-1003-for-e2e-test
spec:
  parentNodeNetworkInterfaces:
    labelSelector:
      matchLabels:
        network.deckhouse.io/interface-type: NIC
        network.deckhouse.io/node-role: worker
  type: VLAN
  vlan:
    id: 1003
EOF`
)

func IsSdnModuleEnabled(f *framework.Framework) bool {
	GinkgoHelper()

	sdnModule, err := f.GetModuleConfig("sdn")
	Expect(err).NotTo(HaveOccurred())
	enabled := sdnModule.Spec.Enabled

	return enabled != nil && *enabled
}

// IsSdnModuleEnabledOrError returns whether SDN module is enabled, or an error if config cannot be read.
// Used by legacy tests that do not have a framework instance.
func IsSdnModuleEnabledOrError() (bool, error) {
	f := framework.NewFramework("")
	sdnModule, err := f.GetModuleConfig("sdn")
	if err != nil {
		return false, err
	}
	enabled := sdnModule.Spec.Enabled
	return enabled != nil && *enabled, nil
}

func IsClusterNetworkExists(f *framework.Framework) bool {
	GinkgoHelper()

	gvr := schema.GroupVersionResource{
		Group:    "network.deckhouse.io",
		Version:  "v1alpha1",
		Resource: "clusternetworks",
	}

	_, err := framework.GetClients().DynamicClient().Resource(gvr).Get(context.Background(), ClusterNetworkName, metav1.GetOptions{})
	Expect(err).To(SatisfyAny(BeNil(), WithTransform(k8serrors.IsNotFound, BeTrue())))

	return err == nil || !k8serrors.IsNotFound(err)
}
