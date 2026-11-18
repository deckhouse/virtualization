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

package rewrite_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"

	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
)

func TestRewrite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Internal resource client")
}

var _ = Describe("Listing rewritten migrations", func() {
	DescribeTable("reads the internal group and respects namespace scope", func(namespace string, expected []string) {
		gvr := (rewrite.VirtualMachineInstanceMigration{}).GVR()

		objects := []runtime.Object{}
		for _, ns := range []string{"source", "target"} {
			objects = append(objects, &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "internal.virtualization.deckhouse.io/v1",
				"kind":       "InternalVirtualizationVirtualMachineInstanceMigration",
				"metadata":   map[string]any{"name": "move", "namespace": ns, "uid": ns + "-uid"},
				"spec":       map[string]any{"vmiName": "guest"},
				"status":     map[string]any{"phase": "Running"},
			}})
		}

		dynamic := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{gvr: "InternalVirtualizationVirtualMachineInstanceMigrationList"}, objects...)
		cl := rewrite.NewRewriteClient(dynamic)
		list := &rewrite.VirtualMachineInstanceMigrationList{}
		Expect(cl.List(context.Background(), list, rewrite.InNamespace(namespace))).To(Succeed())

		namespaces := []string{}
		for _, item := range list.Items {
			namespaces = append(namespaces, item.Namespace)
			Expect(item.UID).To(BeEquivalentTo(item.Namespace + "-uid"))
			Expect(item.Spec.VMIName).To(Equal("guest"))
			Expect(string(item.Status.Phase)).To(Equal("Running"))
		}

		Expect(namespaces).To(ConsistOf(expected))

		actions := dynamic.Actions()
		Expect(actions).To(HaveLen(1))
		Expect(actions[0].GetResource()).To(Equal(gvr))
		Expect(actions[0].GetNamespace()).To(Equal(namespace))
	}, Entry("one namespace", "source", []string{"source"}), Entry("all namespaces", metav1.NamespaceAll, []string{"source", "target"}))
})
