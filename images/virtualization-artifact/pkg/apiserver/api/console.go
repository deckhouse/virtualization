package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	virtv2 "github.com/deckhouse/virtualization-controller/api/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/tools/cache"
	//"k8s.io/kubernetes/pkg/capabilities"
	"net/http"
	"net/url"
	"os"
	"path"
)

type console struct {
	groupResource schema.GroupResource
	vmLister      cache.GenericLister
	kubevirt      KubevirtApiServerConfig
}

var _ rest.KindProvider = &console{}
var _ rest.Storage = &console{}
var _ rest.Connecter = &console{}
var _ rest.Scoper = &console{}

func newConsole(groupResource schema.GroupResource, vmLister cache.GenericLister, kubevirt KubevirtApiServerConfig) *console {
	return &console{
		groupResource: groupResource,
		vmLister:      vmLister,
		kubevirt:      kubevirt,
	}
}

// New implements rest.Storage interface
func (c console) New() runtime.Object {
	return &virtv2.VirtualMachineConsole{}
}

// Destroy implements rest.Storage interface
func (c console) Destroy() {
}

// Kind implements rest.KindProvider interface
func (c console) Kind() string {
	return virtv2.VirtualMachineConsoleKind
}

// NamespaceScoped implements rest.Scoper interface
func (c console) NamespaceScoped() bool {
	return true
}

// Connect implements rest.Connecter interface
func (c console) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	consoleOpts, ok := opts.(*virtv2.VirtualMachineConsole)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	location, transport, err := ConsoleLocation(ctx, c.vmLister, name, consoleOpts, c.kubevirt)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, false, true, responder)
	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (c console) NewConnectOptions() (runtime.Object, bool, string) {
	return &virtv2.VirtualMachineConsole{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (c console) ConnectMethods() []string {
	return upgradeableMethods
}

var upgradeableMethods = []string{http.MethodGet, http.MethodPost}

func ConsoleLocation(
	ctx context.Context,
	getter cache.GenericLister,
	name string,
	opts *virtv2.VirtualMachineConsole,
	kubevirt KubevirtApiServerConfig,
) (*url.URL, http.RoundTripper, error) {
	return streamLocation(ctx, getter, name, opts, "console", kubevirt)
}

func streamLocation(
	ctx context.Context,
	getter cache.GenericLister,
	name string,
	opts runtime.Object,
	streamPath string,
	kubevirt KubevirtApiServerConfig,
) (*url.URL, http.RoundTripper, error) {
	ns, _ := request.NamespaceFrom(ctx)
	vm, err := getVM(getter, types.NamespacedName{Namespace: ns, Name: name})
	if err != nil {
		return nil, nil, err
	}

	if vm.Status.Phase != virtv2.MachineRunning {
		return nil, nil, fmt.Errorf("VirtualMachine is not Running")
	}

	params := url.Values{}
	if err := streamParams(params, opts); err != nil {
		return nil, nil, err
	}

	location := &url.URL{
		Scheme:   "https",
		Host:     kubevirt.Endpoint,
		Path:     fmt.Sprintf("/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachine/%s/%s", vm.Namespace, name, streamPath),
		RawQuery: params.Encode(),
	}
	ca, err := os.ReadFile(path.Join(kubevirt.CertsPath, "ca.crt"))
	if err != nil {
		return nil, nil, err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(ca)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
		RootCAs:            caCertPool,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return location, transport, nil
}

func getVM(getter cache.GenericLister, key types.NamespacedName) (*virtv2.VirtualMachine, error) {
	obj, err := getter.ByNamespace(key.Namespace).Get(key.Name)
	if err != nil {
		return nil, err
	}
	vm := obj.(*virtv2.VirtualMachine)
	if vm == nil {
		return nil, fmt.Errorf("Unexpected object type: %#v", vm)
	}
	return vm, nil
}

func streamParams(_ url.Values, opts runtime.Object) error {
	switch opts := opts.(type) {
	case *virtv2.VirtualMachineConsole:
		return nil
	default:
		return fmt.Errorf("Unknown object for streaming: %v", opts)
	}
	return nil
}

func newThrottledUpgradeAwareProxyHandler(location *url.URL, transport http.RoundTripper, wrapTransport, upgradeRequired bool, responder rest.Responder) http.Handler {
	handler := proxy.NewUpgradeAwareHandler(location, transport, wrapTransport, upgradeRequired, proxy.NewErrorResponder(responder))
	//handler.MaxBytesPerSec = capabilities.Get().PerConnectionBandwidthLimitBytesPerSec
	return handler
}
