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

package peering

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
)

func TestPeering(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Peering migration helpers")
}

var _ = Describe("Recovery migration isolation", func() {
	migration := func(namespace, vmName, phase string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "internal.virtualization.deckhouse.io/v1",
			"kind":       "InternalVirtualizationVirtualMachineInstanceMigration",
			"metadata":   map[string]any{"name": "move", "namespace": namespace},
			"spec":       map[string]any{"vmiName": vmName, "sendTo": map[string]any{"migrationID": "peer-id", "connectURL": "127.0.0.1:9185"}},
			"status":     map[string]any{"phase": phase},
		}}
	}
	newClient := func(objects ...runtime.Object) *fake.FakeDynamicClient {
		gvr := (rewrite.VirtualMachineInstanceMigration{}).GVR()

		return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
			map[schema.GroupVersionResource]string{gvr: "InternalVirtualizationVirtualMachineInstanceMigrationList"}, objects...)
	}

	It("allows preflight on a cluster without migrations", func() {
		Expect(CheckMigrationIsolation(context.Background(), rewrite.NewRewriteClient(newClient()), "")).To(Succeed())
	})

	It("allows active migrations belonging to this test in both namespaces", func() {
		c := newClient(migration("source", "guest", "Running"), migration("target", "guest", "WaitingForSync"))
		Expect(CheckMigrationIsolation(context.Background(), rewrite.NewRewriteClient(c), "guest", "source", "target")).To(Succeed())
	})

	DescribeTable("allows finished foreign migrations", func(phase string) {
		c := newClient(migration("foreign", "other-guest", phase))
		Expect(CheckMigrationIsolation(context.Background(), rewrite.NewRewriteClient(c), "")).To(Succeed())
	}, Entry("succeeded", "Succeeded"), Entry("failed", "Failed"))

	DescribeTable("refuses a restart that would affect another migration", func(namespace, vmName, phase string, deleting bool) {
		m := migration(namespace, vmName, phase)

		if deleting {
			now := metav1.Now()
			m.SetDeletionTimestamp(&now)
		}

		c := newClient(m)
		err := CheckMigrationIsolation(context.Background(), rewrite.NewRewriteClient(c), "guest", "source", "target")
		Expect(err).To(MatchError(And(ContainSubstring(namespace+"/move"), ContainSubstring(vmName), ContainSubstring(phase), ContainSubstring("dedicated cluster"))))
	},
		Entry("a VM with the same name in another namespace", "foreign", "guest", "Running", false),
		Entry("another VM in a test namespace", "source", "other-guest", "Running", false),
		Entry("a pending migration", "foreign", "other-guest", "Pending", false),
		Entry("an unfinished migration being deleted", "foreign", "other-guest", "Running", true),
	)

	It("refuses preflight while any active peer migration exists", func() {
		c := newClient(migration("source", "guest", "Running"))
		Expect(CheckMigrationIsolation(context.Background(), rewrite.NewRewriteClient(c), "")).NotTo(Succeed())
	})

	DescribeTable("allows local migrations that do not use the synchronization controller", func(phase string) {
		m := migration("foreign", "other-guest", phase)
		unstructured.RemoveNestedField(m.Object, "spec", "sendTo")
		Expect(unstructured.SetNestedSlice(m.Object, []any{"127.0.0.1:9185"}, "status", "synchronizationAddresses")).To(Succeed())
		c := newClient(m)
		Expect(CheckMigrationIsolation(context.Background(), rewrite.NewRewriteClient(c), "")).To(Succeed())
	}, Entry("pending", "Pending"), Entry("running", "Running"))

	It("refuses a foreign receiving migration", func() {
		m := migration("foreign", "other-guest", "WaitingForSync")
		unstructured.RemoveNestedField(m.Object, "spec", "sendTo")
		Expect(unstructured.SetNestedField(m.Object, map[string]any{"migrationID": "peer-id"}, "spec", "receive")).To(Succeed())
		c := newClient(m)
		Expect(CheckMigrationIsolation(context.Background(), rewrite.NewRewriteClient(c), "")).To(MatchError(ContainSubstring("foreign/move")))
	})

	It("propagates API errors instead of assuming the cluster is isolated", func() {
		c := newClient()
		unavailable := errors.New("API unavailable")

		c.PrependReactor("list", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, unavailable
		})
		err := CheckMigrationIsolation(context.Background(), rewrite.NewRewriteClient(c), "")
		Expect(errors.Is(err, unavailable)).To(BeTrue())
	})
})
