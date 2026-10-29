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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func clusterPeer(phase string, capabilities []any, endpoint string) *unstructured.Unstructured {
	cp := &unstructured.Unstructured{Object: map[string]any{"status": map[string]any{
		"phase":                 phase,
		"effectiveCapabilities": capabilities,
		"access": map[string]any{"endpoint": endpoint, "caBundle": "PEM", "audiences": []any{
			map[string]any{"capability": "mesh", "audience": "peering.deckhouse.io/peer/uuid-b/cap/mesh"},
			map[string]any{"capability": Capability, "audience": "peering.deckhouse.io/peer/uuid-b/cap/virtualization"},
		}},
	}}}
	cp.SetGroupVersionKind(clusterPeerGVK)
	cp.SetName("cluster-b")
	cp.SetResourceVersion("7")
	return cp
}

var _ = Describe("PeerClients", func() {
	It("connects through the tunnel of an active peer with the token for the audience of its virtualization capability", func() {
		cfg, audience, source, err := clusterPeerConfig(clusterPeer("Active", []any{"mesh", Capability}, "https://peer-cluster-b.d8-peering.svc"))
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Host).To(Equal("https://peer-cluster-b.d8-peering.svc"))
		Expect(cfg.BearerToken).To(BeEmpty())
		Expect(cfg.BearerTokenFile).To(BeEmpty())
		Expect(audience).To(Equal("peering.deckhouse.io/peer/uuid-b/cap/virtualization"))
		Expect(string(cfg.TLSClientConfig.CAData)).To(Equal("PEM"))
		Expect(source).To(HavePrefix("clusterpeer/"))

		// A status update alone must keep the client: only the connection parameters rebuild it.
		same := clusterPeer("Active", []any{"mesh", Capability}, "https://peer-cluster-b.d8-peering.svc")
		same.SetResourceVersion("8")
		_, _, sameSource, err := clusterPeerConfig(same)
		Expect(err).NotTo(HaveOccurred())
		Expect(sameSource).To(Equal(source))

		other := clusterPeer("Active", []any{"mesh", Capability}, "https://peer-cluster-c.d8-peering.svc")
		_, _, otherSource, err := clusterPeerConfig(other)
		Expect(err).NotTo(HaveOccurred())
		Expect(otherSource).NotTo(Equal(source))
	})

	It("requests a token per audience and renews it before it expires", func() {
		p := &PeerClients{tokens: map[string]peerToken{}}
		issued := 0
		expires := time.Now().Add(time.Hour)
		p.issueToken = func(_ context.Context, audience string) (string, time.Time, error) {
			issued++
			return fmt.Sprintf("%s#%d", audience, issued), expires, nil
		}
		ctx := context.Background()
		first, err := p.token(ctx, "aud-b")
		Expect(err).NotTo(HaveOccurred())
		again, err := p.token(ctx, "aud-b")
		Expect(err).NotTo(HaveOccurred())
		Expect(again).To(Equal(first), "a live token is reused")
		other, err := p.token(ctx, "aud-c")
		Expect(err).NotTo(HaveOccurred())
		Expect(other).NotTo(Equal(first), "each audience gets its own token")

		expires = time.Now().Add(tokenRefresh / 2)
		p.tokens["aud-b"] = peerToken{token: first, expires: expires}
		renewed, err := p.token(ctx, "aud-b")
		Expect(err).NotTo(HaveOccurred())
		Expect(renewed).NotTo(Equal(first), "a token about to expire is replaced")
	})

	DescribeTable("refuses a peer that cannot receive",
		func(cp *unstructured.Unstructured) {
			_, _, _, err := clusterPeerConfig(cp)
			Expect(errors.Is(err, ErrPeerNotSupported)).To(BeTrue())
		},
		Entry("not active", clusterPeer("Pending", []any{Capability}, "https://x")),
		Entry("no capability", clusterPeer("Active", []any{"mesh"}, "https://x")),
		Entry("no endpoint", clusterPeer("Active", []any{Capability}, "")),
		Entry("no audience for virtualization", func() *unstructured.Unstructured {
			cp := clusterPeer("Active", []any{Capability}, "https://x")
			Expect(unstructured.SetNestedSlice(cp.Object, []any{}, "status", "access", "audiences")).To(Succeed())
			return cp
		}()),
	)

	It("moves to the namespace of the same name with the own address, unless a dev Secret says otherwise", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: DevPeerSecretPrefix + "self", Namespace: "d8-virtualization"},
			Data:       map[string][]byte{"namespaces": []byte("src=dst, other=x"), "newAddress": []byte("true")},
		}
		p, err := NewPeerClients(fake.NewClientBuilder().WithObjects(secret).Build(), nil, "d8-virtualization")
		Expect(err).NotTo(HaveOccurred())

		target, err := p.Target(context.Background(), "cluster-b", "src")
		Expect(err).NotTo(HaveOccurred())
		Expect(target).To(Equal(Target{Namespace: "src"}))
		target, err = p.Target(context.Background(), "self", "src")
		Expect(err).NotTo(HaveOccurred())
		Expect(target).To(Equal(Target{Namespace: "dst", NewAddress: true}))
	})

	It("tells whether the project of a namespace is paired with a peer that offers virtualization", func() {
		projectPeer := func(name, state string, capabilities []any) *unstructured.Unstructured {
			pp := &unstructured.Unstructured{Object: map[string]any{"status": map[string]any{"state": state, "capabilities": capabilities}}}
			pp.SetGroupVersionKind(projectPeerGVK)
			pp.SetNamespace("app-x")
			pp.SetName(name)
			return pp
		}
		virt := []any{map[string]any{"name": Capability, "version": "1.2.0"}}
		cl := fake.NewClientBuilder().WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-x", Labels: map[string]string{labelProject: "app-x"}}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-x-extra", Labels: map[string]string{labelProject: "app-x"}}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "orphan"}},
			projectPeer("cluster-b", "Paired", virt),
			projectPeer("cluster-c", "PeerUnreachable", virt),
			projectPeer("cluster-d", "Paired", []any{map[string]any{"name": "mesh"}}),
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: DevPeerSecretPrefix + "self", Namespace: "d8-virtualization"}},
		).Build()
		p, err := NewPeerClients(cl, nil, "d8-virtualization")
		Expect(err).NotTo(HaveOccurred())
		ctx := context.Background()

		Expect(p.Paired(ctx, "cluster-b", "app-x")).To(Succeed())
		Expect(p.Paired(ctx, "cluster-b", "app-x-extra")).To(Succeed(), "an additional namespace of the project")
		Expect(p.Paired(ctx, "self", "orphan")).To(Succeed(), "a dev Secret stands for a pair")
		for peer, ns := range map[string]string{"cluster-c": "app-x", "cluster-d": "app-x", "cluster-e": "app-x", "cluster-b": "orphan"} {
			Expect(errors.Is(p.Paired(ctx, peer, ns), ErrPeerNotPaired)).To(BeTrue(), "%s in %s", peer, ns)
		}
	})

	It("prefers the kubeconfig of a dev Secret", func() {
		kubeconfig := `apiVersion: v1
kind: Config
clusters: [{name: b, cluster: {server: "https://api.b.example"}}]
users: [{name: b, user: {token: "t"}}]
contexts: [{name: b, context: {cluster: b, user: b}}]
current-context: b
`
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: DevPeerSecretPrefix + "cluster-b", Namespace: "d8-virtualization", ResourceVersion: "3"},
			Data:       map[string][]byte{"kubeconfig": []byte(kubeconfig)},
		}
		reader := fake.NewClientBuilder().WithObjects(secret).Build()
		p, err := NewPeerClients(reader, nil, "d8-virtualization")
		Expect(err).NotTo(HaveOccurred())
		cfg, _, _, err := p.config(context.Background(), "cluster-b")
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Host).To(Equal("https://api.b.example"))
		Expect(cfg.BearerToken).To(Equal("t"))
	})

	It("reads the peer again only after a while", func() {
		kubeconfig := `apiVersion: v1
kind: Config
clusters: [{name: b, cluster: {server: "https://api.b.example"}}]
users: [{name: b, user: {token: "t"}}]
contexts: [{name: b, context: {cluster: b, user: b}}]
current-context: b
`
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: DevPeerSecretPrefix + "cluster-b", Namespace: "d8-virtualization"},
			Data:       map[string][]byte{"kubeconfig": []byte(kubeconfig), "namespaces": []byte("src=dst")},
		}
		reader := fake.NewClientBuilder().WithObjects(secret).Build()
		p, err := NewPeerClients(reader, nil, "d8-virtualization")
		Expect(err).NotTo(HaveOccurred())
		ctx := context.Background()
		first, err := p.Client(ctx, "cluster-b")
		Expect(err).NotTo(HaveOccurred())

		Expect(reader.Delete(ctx, secret)).To(Succeed())
		again, err := p.Client(ctx, "cluster-b")
		Expect(err).NotTo(HaveOccurred())
		Expect(again).To(BeIdenticalTo(first))
		target, err := p.Target(ctx, "cluster-b", "src")
		Expect(err).NotTo(HaveOccurred())
		Expect(target.Namespace).To(Equal("dst"))

		c := p.clients["cluster-b"]
		c.checked = time.Now().Add(-peerRecheck)
		p.clients["cluster-b"] = c
		_, err = p.Client(ctx, "cluster-b")
		Expect(errors.Is(err, ErrPeerNotSupported)).To(BeTrue())
	})

	It("reports a peer without a ClusterPeer as not supported", func() {
		p, err := NewPeerClients(fake.NewClientBuilder().Build(), nil, "d8-virtualization")
		Expect(err).NotTo(HaveOccurred())
		_, _, _, err = p.config(context.Background(), "cluster-x")
		Expect(errors.Is(err, ErrPeerNotSupported)).To(BeTrue())
	})
})
