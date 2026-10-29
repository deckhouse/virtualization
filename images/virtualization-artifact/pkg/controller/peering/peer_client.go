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
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
)

const (
	// Capability is the name of the capability of this module in the peering.
	Capability = "virtualization"
	// serviceAccountName is the own ServiceAccount of virtualization-controller: its tokens, issued for the
	// audience a peer publishes, are what the peering tunnel presents to that peer.
	serviceAccountName = "virtualization-controller"
	tokenTTL           = time.Hour
	// tokenRefresh is how long before its expiry a token is replaced.
	tokenRefresh = 10 * time.Minute
	// peerRecheck is how long a client is used before the ClusterPeer or the dev Secret of its peer is read
	// again: the operations of a move ask for the peer every few seconds.
	peerRecheck = 30 * time.Second
	// DevPeerSecretPrefix names a Secret in the module namespace with a kubeconfig of a peer: for stands
	// without the peering module only. The kubeconfig must act as virtualization-controller of the
	// peer, the identity its webhook lets create requests to receive.
	DevPeerSecretPrefix = "peering-dev-peer-"
	devPeerSecretKey    = "kubeconfig"
	// devPeerNamespacesKey maps namespaces here to the ones of the peer, "a=b,c=d", and devPeerNewAddressKey
	// gives the VM an address of the peer: a cluster that is its own peer moves a VM between namespaces and
	// cannot keep its address.
	devPeerNamespacesKey = "namespaces"
	devPeerNewAddressKey = "newAddress"
)

// Target is where a VM moves in the peer.
type Target struct {
	// Namespace of the VM in the peer: the paired project has the same name.
	Namespace string
	// NewAddress gives the VM an address of the peer instead of keeping its own.
	NewAddress bool
}

// ErrPeerNotSupported tells that the peer cannot be used to move virtual machines: it is unknown, not
// active or has no virtualization capability.
var ErrPeerNotSupported = errors.New("the peer cluster does not support virtualization")

// ErrPeerNotPaired tells that the project is not paired with the peer or the peer offers no virtualization
// to it.
var ErrPeerNotPaired = errors.New("the peer cluster is not paired with this project")

// labelProject binds a namespace to its project; the main namespace of a project is named after it.
const labelProject = "projects.deckhouse.io/project"

var (
	clusterPeerGVK = schema.GroupVersionKind{Group: "peering.deckhouse.io", Version: "v1alpha1", Kind: "ClusterPeer"}
	projectPeerGVK = schema.GroupVersionKind{Group: "peering.deckhouse.io", Version: "v1alpha1", Kind: "ProjectPeer"}
)

// PeerClients gives clients to the apiservers of peer clusters, as the peering module lets consumers
// reach them: through the local tunnel Service from ClusterPeer.status.access, with a token of the own
// ServiceAccount for the audience the peer publishes for the virtualization capability. The audience
// names the peer, so the token is requested per peer and renewed before it expires. The clients are kept
// while their source stays the same.
type PeerClients struct {
	reader    client.Reader
	namespace string
	scheme    *runtime.Scheme
	// issueToken requests a token of the own ServiceAccount for an audience.
	issueToken func(ctx context.Context, audience string) (string, time.Time, error)

	mu      sync.Mutex
	clients map[string]peerClient
	tokens  map[string]peerToken
}

type peerClient struct {
	source    string
	client    client.Client
	transport *http.Transport
	// dev is the data of the dev Secret of the peer, nil without one.
	dev     map[string][]byte
	checked time.Time
}

type peerToken struct {
	token   string
	expires time.Time
}

// NewPeerClients reads the peers through reader; tokens creates the tokens of the own ServiceAccount.
func NewPeerClients(reader client.Reader, tokens client.Client, namespace string) (*PeerClients, error) {
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, storagev1.AddToScheme, v1alpha2.AddToScheme, intvirtv1alpha1.AddToScheme} {
		if err := add(scheme); err != nil {
			return nil, err
		}
	}
	p := &PeerClients{reader: reader, namespace: namespace, scheme: scheme, clients: map[string]peerClient{}, tokens: map[string]peerToken{}}
	p.issueToken = func(ctx context.Context, audience string) (string, time.Time, error) {
		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: serviceAccountName}}
		req := &authnv1.TokenRequest{Spec: authnv1.TokenRequestSpec{
			Audiences:         []string{audience},
			ExpirationSeconds: ptr.To(int64(tokenTTL.Seconds())),
		}}
		if err := tokens.SubResource("token").Create(ctx, sa, req); err != nil {
			return "", time.Time{}, err
		}
		return req.Status.Token, req.Status.ExpirationTimestamp.Time, nil
	}
	return p, nil
}

// token is the token for the audience, requested anew shortly before the one in hand expires.
func (p *PeerClients) token(ctx context.Context, audience string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if t, ok := p.tokens[audience]; ok && time.Until(t.expires) > tokenRefresh {
		return t.token, nil
	}
	token, expires, err := p.issueToken(ctx, audience)
	if err != nil {
		return "", fmt.Errorf("token for the audience %q: %w", audience, err)
	}
	p.tokens[audience] = peerToken{token: token, expires: expires}
	return token, nil
}

// bearer puts the token for the audience on every request.
type bearer struct {
	next  http.RoundTripper
	token func(context.Context) (string, error)
}

func (b *bearer) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := b.token(req.Context())
	if err != nil {
		return nil, err
	}
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+token)
	return b.next.RoundTrip(req)
}

// Client returns a client to the apiserver of the peer.
func (p *PeerClients) Client(ctx context.Context, peer string) (client.Client, error) {
	p.mu.Lock()
	cached, ok := p.clients[peer]
	p.mu.Unlock()
	if ok && time.Since(cached.checked) < peerRecheck {
		return cached.client, nil
	}

	cfg, source, dev, err := p.config(ctx, peer)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[peer]; ok && c.source == source {
		c.dev, c.checked = dev, time.Now()
		p.clients[peer] = c
		return c.client, nil
	}

	var transport *http.Transport
	wrap := cfg.WrapTransport
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		if t, ok := rt.(*http.Transport); ok {
			transport = t
		}
		if wrap != nil {
			return wrap(rt)
		}
		return rt
	}
	c, err := client.New(cfg, client.Options{Scheme: p.scheme})
	if err != nil {
		if transport != nil {
			transport.CloseIdleConnections()
		}
		return nil, fmt.Errorf("client to the peer %q: %w", peer, err)
	}
	if old, ok := p.clients[peer]; ok && old.transport != nil {
		old.transport.CloseIdleConnections()
	}
	p.clients[peer] = peerClient{source: source, client: c, transport: transport, dev: dev, checked: time.Now()}
	return c, nil
}

// config returns the connection to the peer and a version of its source, to know when to rebuild it.
func (p *PeerClients) config(ctx context.Context, peer string) (*rest.Config, string, map[string][]byte, error) {
	var secret corev1.Secret
	err := p.reader.Get(ctx, types.NamespacedName{Namespace: p.namespace, Name: DevPeerSecretPrefix + peer}, &secret)
	if err == nil {
		cfg, err := clientcmd.RESTConfigFromKubeConfig(secret.Data[devPeerSecretKey])
		if err != nil {
			return nil, "", nil, fmt.Errorf("the kubeconfig of the peer %q in Secret %s: %w", peer, secret.Name, err)
		}
		return cfg, "secret/" + secret.ResourceVersion, secret.Data, nil
	}
	if !k8serrors.IsNotFound(err) {
		return nil, "", nil, err
	}

	cp := &unstructured.Unstructured{}
	cp.SetGroupVersionKind(clusterPeerGVK)
	if err = p.reader.Get(ctx, types.NamespacedName{Name: peer}, cp); err != nil {
		if k8serrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil, "", nil, fmt.Errorf("%w: no ClusterPeer %q", ErrPeerNotSupported, peer)
		}
		return nil, "", nil, err
	}
	cfg, audience, source, err := clusterPeerConfig(cp)
	if err != nil {
		return nil, "", nil, err
	}
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return &bearer{next: rt, token: func(ctx context.Context) (string, error) { return p.token(ctx, audience) }}
	}
	return cfg, source, nil, nil
}

// Paired tells whether the project of the namespace is paired with the peer and the peer offers
// virtualization to it: the peering module keeps a ProjectPeer per paired peer in the main namespace
// of the project, named after the peer, with the state of the pair and its capabilities. A dev Secret
// for the peer stands for a pair.
func (p *PeerClients) Paired(ctx context.Context, peer, namespace string) error {
	var secret corev1.Secret
	err := p.reader.Get(ctx, types.NamespacedName{Namespace: p.namespace, Name: DevPeerSecretPrefix + peer}, &secret)
	if err == nil {
		return nil
	}
	if !k8serrors.IsNotFound(err) {
		return err
	}
	// An additional namespace of a project (<project>-<name>) carries the project in a label; the
	// ProjectPeer lives in the main namespace, named after the project.
	var ns corev1.Namespace
	if err = p.reader.Get(ctx, types.NamespacedName{Name: namespace}, &ns); err != nil {
		return err
	}
	project := ns.Labels[labelProject]
	if project == "" {
		return fmt.Errorf("%w: namespace %q belongs to no project", ErrPeerNotPaired, namespace)
	}
	pp := &unstructured.Unstructured{}
	pp.SetGroupVersionKind(projectPeerGVK)
	if err = p.reader.Get(ctx, types.NamespacedName{Namespace: project, Name: peer}, pp); err != nil {
		if k8serrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return fmt.Errorf("%w: no ProjectPeer %q in project %q", ErrPeerNotPaired, peer, project)
		}
		return err
	}
	if state, _, _ := unstructured.NestedString(pp.Object, "status", "state"); state != "Paired" {
		return fmt.Errorf("%w: ProjectPeer %q is %q, not Paired", ErrPeerNotPaired, peer, state)
	}
	capabilities, _, _ := unstructured.NestedSlice(pp.Object, "status", "capabilities")
	for _, c := range capabilities {
		if m, ok := c.(map[string]any); ok && m["name"] == Capability {
			return nil
		}
	}
	return fmt.Errorf("%w: ProjectPeer %q offers no %s capability", ErrPeerNotPaired, peer, Capability)
}

// Target returns where a VM of the namespace moves in the peer: the namespace of the same name and the own
// address, unless a dev Secret of the peer says otherwise.
func (p *PeerClients) Target(ctx context.Context, peer, namespace string) (Target, error) {
	target := Target{Namespace: namespace}
	p.mu.Lock()
	c, ok := p.clients[peer]
	p.mu.Unlock()
	dev := c.dev
	if !ok {
		var secret corev1.Secret
		err := p.reader.Get(ctx, types.NamespacedName{Namespace: p.namespace, Name: DevPeerSecretPrefix + peer}, &secret)
		if client.IgnoreNotFound(err) != nil {
			return target, err
		}
		dev = secret.Data
	}
	if dev == nil {
		return target, nil
	}
	for _, pair := range strings.Split(string(dev[devPeerNamespacesKey]), ",") {
		from, to, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if ok && from == namespace && to != "" {
			target.Namespace = to
		}
	}
	target.NewAddress = string(dev[devPeerNewAddressKey]) == "true"
	return target, nil
}

// clusterPeerConfig builds the connection from ClusterPeer.status: the peer must be active, offer the
// virtualization capability both ways and publish the audience of the tokens it accepts for it. The
// config carries no credential: the token for the audience is put on each request.
func clusterPeerConfig(cp *unstructured.Unstructured) (cfg *rest.Config, audience, source string, err error) {
	phase, _, _ := unstructured.NestedString(cp.Object, "status", "phase")
	if phase != "Active" {
		return nil, "", "", fmt.Errorf("%w: ClusterPeer %q is %q, not Active", ErrPeerNotSupported, cp.GetName(), phase)
	}
	capabilities, _, _ := unstructured.NestedStringSlice(cp.Object, "status", "effectiveCapabilities")
	if !slices.Contains(capabilities, Capability) {
		return nil, "", "", fmt.Errorf("%w: ClusterPeer %q has no %s capability", ErrPeerNotSupported, cp.GetName(), Capability)
	}
	endpoint, _, _ := unstructured.NestedString(cp.Object, "status", "access", "endpoint")
	ca, _, _ := unstructured.NestedString(cp.Object, "status", "access", "caBundle")
	if endpoint == "" {
		return nil, "", "", fmt.Errorf("%w: ClusterPeer %q has no access endpoint", ErrPeerNotSupported, cp.GetName())
	}
	audiences, _, _ := unstructured.NestedSlice(cp.Object, "status", "access", "audiences")
	for _, a := range audiences {
		if m, ok := a.(map[string]any); ok && m["capability"] == Capability {
			audience, _ = m["audience"].(string)
		}
	}
	if audience == "" {
		return nil, "", "", fmt.Errorf("%w: ClusterPeer %q publishes no audience for the %s capability", ErrPeerNotSupported, cp.GetName(), Capability)
	}
	// The source is the connection, not the resource version: a status update that leaves the endpoint,
	// the CA and the audience alone must not rebuild the client and leak its connections.
	sum := sha256.Sum256([]byte(endpoint + "\x00" + ca + "\x00" + audience))
	return &rest.Config{
		Host:            endpoint,
		TLSClientConfig: rest.TLSClientConfig{CAData: []byte(ca)},
	}, audience, fmt.Sprintf("clusterpeer/%x", sum), nil
}
