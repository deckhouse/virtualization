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

package migrationiface

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
)

const (
	testSystemNetworkName = "migration"
	testNode              = "node-1"
	testIfName            = "eth0.999"
	testNNIName           = "node-1-migration-vlan-999-abcdef"
)

func newFakeClient(objs ...client.Object) client.WithWatch {
	GinkgoHelper()
	scheme := runtime.NewScheme()
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())

	b := fake.NewClientBuilder().WithScheme(scheme)
	for _, fn := range indexer.IndexGettersSDN {
		b.WithIndex(fn())
	}
	if len(objs) > 0 {
		b.WithObjects(objs...)
	}
	return b.Build()
}

func newNode(annotationVal string) *corev1.Node {
	n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: testNode}}
	if annotationVal != "" {
		n.Annotations = map[string]string{annotations.AnnMigrationIface: annotationVal}
	}
	return n
}

func newSNNNIA(name, sn, nodeName, nniName string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(snnniaGVK)
	u.SetName(name)
	_ = unstructured.SetNestedField(u.Object, sn, "spec", "systemNetworkName")
	if nodeName != "" {
		_ = unstructured.SetNestedField(u.Object, nodeName, "status", "nodeName")
	}
	if nniName != "" {
		_ = unstructured.SetNestedField(u.Object, nniName, "status", "nodeNetworkInterfaceName")
	}
	return u
}

func newNNI(ifName string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(nniGVK)
	u.SetName(testNNIName)
	if ifName != "" {
		_ = unstructured.SetNestedField(u.Object, ifName, "status", "ifName")
	}
	return u
}

var _ = Describe("Reconciler", func() {
	var (
		ctx context.Context
		r   *Reconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	reconcileNode := func(c client.Client, nodeName string) {
		GinkgoHelper()
		r = NewReconciler(c, testSystemNetworkName, log.NewNop())
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKey{Name: nodeName}})
		Expect(err).NotTo(HaveOccurred())
	}

	getNode := func(c client.Client) *corev1.Node {
		GinkgoHelper()
		n := &corev1.Node{}
		Expect(c.Get(context.Background(), client.ObjectKey{Name: testNode}, n)).To(Succeed())
		return n
	}

	It("sets the annotation from SNNNIA + NNI when the node has none", func() {
		c := newFakeClient(
			newNode(""),
			newSNNNIA("attachment-1", testSystemNetworkName, testNode, testNNIName),
			newNNI(testIfName),
		)
		reconcileNode(c, testNode)
		Expect(getNode(c).Annotations[annotations.AnnMigrationIface]).To(Equal(testIfName))
	})

	It("does not patch when annotation already matches the resolved value", func() {
		node := newNode(testIfName)
		c := newFakeClient(
			node,
			newSNNNIA("attachment-1", testSystemNetworkName, testNode, testNNIName),
			newNNI(testIfName),
		)
		rvBefore := node.ResourceVersion

		reconcileNode(c, testNode)

		got := getNode(c)
		Expect(got.Annotations[annotations.AnnMigrationIface]).To(Equal(testIfName))
		Expect(got.ResourceVersion).To(Equal(rvBefore), "ResourceVersion bump indicates a spurious patch")
	})

	It("clears a stale annotation when no matching SNNNIA exists", func() {
		c := newFakeClient(newNode("stale-iface"))
		reconcileNode(c, testNode)
		_, present := getNode(c).Annotations[annotations.AnnMigrationIface]
		Expect(present).To(BeFalse())
	})

	It("is a no-op when node has no annotation and resolver returns empty", func() {
		c := newFakeClient(newNode(""))
		reconcileNode(c, testNode)
		_, present := getNode(c).Annotations[annotations.AnnMigrationIface]
		Expect(present).To(BeFalse())
	})

	It("returns no error when the Node has been deleted", func() {
		c := newFakeClient()
		r = NewReconciler(c, testSystemNetworkName, log.NewNop())
		_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKey{Name: "nonexistent"}})
		Expect(err).NotTo(HaveOccurred())
	})

	It("skips SNNNIA for a different SystemNetwork", func() {
		c := newFakeClient(
			newNode(""),
			newSNNNIA("attachment-other", "other-sn", testNode, testNNIName),
			newNNI(testIfName),
		)
		reconcileNode(c, testNode)
		_, present := getNode(c).Annotations[annotations.AnnMigrationIface]
		Expect(present).To(BeFalse(), "must not annotate from an attachment belonging to another SystemNetwork")
	})

	It("skips SNNNIA without status.nodeName (IPAM-failed attachments)", func() {
		c := newFakeClient(
			newNode(""),
			newSNNNIA("attachment-ipam-failed", testSystemNetworkName, "", ""),
		)
		reconcileNode(c, testNode)
		_, present := getNode(c).Annotations[annotations.AnnMigrationIface]
		Expect(present).To(BeFalse())
	})

	Describe("resolveInterfaceForNode", func() {
		It("returns empty when the referenced NNI doesn't exist", func() {
			c := newFakeClient(
				newSNNNIA("attachment-1", testSystemNetworkName, testNode, "missing-nni"),
			)
			r = NewReconciler(c, testSystemNetworkName, log.NewNop())
			got, err := r.resolveInterfaceForNode(context.Background(), testNode)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(BeEmpty())
		})

		It("returns empty when the NNI has no status.ifName yet", func() {
			c := newFakeClient(
				newSNNNIA("attachment-1", testSystemNetworkName, testNode, testNNIName),
				newNNI(""),
			)
			r = NewReconciler(c, testSystemNetworkName, log.NewNop())
			got, err := r.resolveInterfaceForNode(context.Background(), testNode)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(BeEmpty())
		})
	})
})
