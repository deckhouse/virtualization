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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	ClusterNetworkName   = "cn-1003-for-e2e-test"
	ClusterNetworkVLANID = 1003
)

func IsSdnModuleEnabled(f *framework.Framework) bool {
	GinkgoHelper()

	sdnModule, err := f.GetModuleConfig("sdn")
	Expect(err).NotTo(HaveOccurred())
	enabled := sdnModule.Spec.Enabled

	return enabled != nil && *enabled
}

func CreateClusterNetworkIfNotExists(f *framework.Framework) {
	GinkgoHelper()

	gvr := schema.GroupVersionResource{
		Group:    "network.deckhouse.io",
		Version:  "v1alpha1",
		Resource: "clusternetworks",
	}

	_, err := framework.GetClients().DynamicClient().Resource(gvr).Get(context.Background(), ClusterNetworkName, metav1.GetOptions{})
	Expect(err).To(SatisfyAny(BeNil(), WithTransform(k8serrors.IsNotFound, BeTrue())))

	if err != nil && k8serrors.IsNotFound(err) {
		unstructuredClusterNetwork := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": gvr.GroupVersion().String(),
				"kind":       "ClusterNetwork",
				"metadata": map[string]any{
					"name": ClusterNetworkName,
				},
			},
		}

		unstructuredClusterNetwork.Object["spec"] = map[string]any{
			"vlan": map[string]any{
				"id": ClusterNetworkVLANID,
			},
			"parentNodeNetworkInterfaces": map[string]any{
				"labelSelector": map[string]any{
					"matchLabels": map[string]any{
						"network.deckhouse.io/interface-type": "NIC",
						"network.deckhouse.io/node-role":      "worker",
					},
				},
			},
			"type": "VLAN",
		}

		_, err = framework.GetClients().DynamicClient().Resource(gvr).Create(context.Background(), unstructuredClusterNetwork, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		UntilConditionStatus("Ready", string(metav1.ConditionTrue), framework.MiddleTimeout, unstructuredClusterNetwork)
		Expect(err).NotTo(HaveOccurred())
	}
}
