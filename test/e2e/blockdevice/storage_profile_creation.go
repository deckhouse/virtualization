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

package blockdevice

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
)

var (
	replicatedStorageClassGVR = schema.GroupVersionResource{
		Group:    "storage.deckhouse.io",
		Version:  "v1alpha1",
		Resource: "replicatedstorageclasses",
	}
	replicatedStoragePoolGVR = schema.GroupVersionResource{
		Group:    "storage.deckhouse.io",
		Version:  "v1alpha1",
		Resource: "replicatedstoragepools",
	}
)

// The virtualization-controller storageprofile controller must create a StorageProfile
// for every StorageClass added to the cluster. This is verified by creating a new
// StorageClass via a ReplicatedStorageClass and waiting for the matching StorageProfile.
var _ = Describe("StorageProfileCreation", Label(precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		// No namespace is required: ReplicatedStorageClass, StorageClass and
		// StorageProfile are all cluster-scoped.
		f = framework.NewFramework("")
		f.Before()
		DeferCleanup(f.After)
	})

	It("creates a StorageProfile when a new StorageClass is added via a ReplicatedStorageClass", func() {
		poolName := discoverReplicatedStoragePool(ctx, f)

		// Cluster-scoped name with a random suffix to stay unique across parallel runs.
		name := fmt.Sprintf("v12n-e2e-storageprofile-%s", rand.String(6))

		By("Creating a test ReplicatedStorageClass", func() {
			_, err := f.DynamicClient().Resource(replicatedStorageClassGVR).Create(ctx, newReplicatedStorageClass(name, poolName), metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "failed to create ReplicatedStorageClass %q", name)

			DeferCleanup(func() {
				err := f.DynamicClient().Resource(replicatedStorageClassGVR).Delete(context.Background(), name, metav1.DeleteOptions{})
				Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue(),
					"failed to delete ReplicatedStorageClass %q: %v", name, err)
			})
		})

		By("Waiting for the StorageClass to be created from the ReplicatedStorageClass", func() {
			Eventually(func() error {
				_, err := f.KubeClient().StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
				return err
			}).WithTimeout(framework.LongTimeout).WithPolling(framework.PollingInterval).Should(Succeed(),
				"StorageClass %q was not created from the ReplicatedStorageClass", name)
		})

		By("Waiting for the StorageProfile to be created for the new StorageClass", func() {
			Eventually(func(g Gomega) {
				sp := &rewrite.StorageProfile{}
				err := f.RewriteClient().Get(ctx, name, sp)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sp.StorageProfile).NotTo(BeNil())
				g.Expect(sp.Status.StorageClass).NotTo(BeNil(), "StorageProfile %q must reference its StorageClass", name)
				g.Expect(*sp.Status.StorageClass).To(Equal(name))
				g.Expect(sp.Status.Provisioner).NotTo(BeNil(), "StorageProfile %q must report its provisioner", name)
			}).WithTimeout(framework.LongTimeout).WithPolling(framework.PollingInterval).Should(Succeed(),
				"StorageProfile %q was not created for the new StorageClass", name)
		})
	})
})

// discoverReplicatedStoragePool returns the name of a usable ReplicatedStoragePool in
// the cluster. The test is skipped when sds-replicated-volume is not installed or no
// storage pool is available.
func discoverReplicatedStoragePool(ctx context.Context, f *framework.Framework) string {
	GinkgoHelper()

	list, err := f.DynamicClient().Resource(replicatedStoragePoolGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		Skip(fmt.Sprintf("ReplicatedStoragePool resources are not available (sds-replicated-volume not installed?): %v", err))
	}
	if len(list.Items) == 0 {
		Skip("no ReplicatedStoragePool found in the cluster")
	}

	for i := range list.Items {
		phase, _, _ := unstructured.NestedString(list.Items[i].Object, "status", "phase")
		if !strings.EqualFold(phase, "Failed") {
			return list.Items[i].GetName()
		}
	}

	return list.Items[0].GetName()
}

func newReplicatedStorageClass(name, storagePool string) *unstructured.Unstructured {
	rsc := &unstructured.Unstructured{}
	rsc.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "storage.deckhouse.io",
		Version: "v1alpha1",
		Kind:    "ReplicatedStorageClass",
	})
	rsc.SetName(name)
	rsc.Object["spec"] = map[string]interface{}{
		"replication":   "None",
		"storagePool":   storagePool,
		"reclaimPolicy": "Delete",
		"volumeAccess":  "Any",
		"topology":      "Ignored",
	}
	return rsc
}
