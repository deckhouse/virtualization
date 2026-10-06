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

package wrapresourceslice

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func TestWrapResourceSlice(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WrapResourceSlice Suite")
}

var _ = Describe("Controller", func() {
	const (
		driverName = "test-driver"
		nodeName   = "node-a"
	)

	It("recreates a slice deleted right after the controller updated it", func(ctx context.Context) {
		// kubelet wipes the node's slices on startup, which can happen right
		// after a restarted driver has updated its slice from the previous run.
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName, UID: "node-uid"}}
		oldSlice := &resourcev1.ResourceSlice{
			ObjectMeta: metav1.ObjectMeta{Name: "old-slice"},
			Spec: resourcev1.ResourceSliceSpec{
				Driver:   driverName,
				NodeName: ptr.To(nodeName),
				Pool:     resourcev1.ResourcePool{Name: nodeName, ResourceSliceCount: 1},
				Devices:  []resourcev1.Device{{Name: "dev-old"}},
			},
		}
		client := fake.NewClientset(node, oldSlice)

		ctrl, err := StartController(ctx, Options{
			DriverName:       driverName,
			KubeClient:       client,
			Owner:            &Owner{APIVersion: "v1", Kind: "Node", Name: nodeName},
			MutationCacheTTL: ptr.To(2 * time.Second),
			SyncDelay:        ptr.To(100 * time.Millisecond),
			Resources: &DriverResources{Pools: map[string]Pool{
				nodeName: {Slices: []Slice{{Devices: []resourcev1.Device{{Name: "dev-new"}}}}},
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(ctrl.Stop)

		slices := func(g Gomega) []resourcev1.ResourceSlice {
			list, err := client.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
			g.Expect(err).NotTo(HaveOccurred())
			return list.Items
		}
		hasNewDevice := func(g Gomega) {
			items := slices(g)
			g.Expect(items).To(HaveLen(1))
			g.Expect(items[0].Spec.Devices).To(ConsistOf(HaveField("Name", "dev-new")))
		}

		Eventually(hasNewDevice).WithContext(ctx).WithTimeout(5 * time.Second).Should(Succeed())

		Expect(client.ResourceV1().ResourceSlices().Delete(ctx, "old-slice", metav1.DeleteOptions{})).To(Succeed())

		Eventually(hasNewDevice).WithContext(ctx).WithTimeout(10 * time.Second).Should(Succeed())
	}, SpecTimeout(30*time.Second))
})
