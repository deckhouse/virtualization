package rest

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	"github.com/deckhouse/virtualization/api/subresources"
)

type ConsoleREST struct {
	vmLister         cache.GenericLister
	proxyCertManager certmanager.CertificateManager
	kubevirt         KubevirtApiServerConfig
}

type KubevirtApiServerConfig struct {
	Endpoint       string
	CaBundlePath   string
	ServiceAccount types.NamespacedName
}

var (
	_ rest.Storage   = &ConsoleREST{}
	_ rest.Connecter = &ConsoleREST{}
)

func NewConsoleREST(vmLister cache.GenericLister, kubevirt KubevirtApiServerConfig, proxyCertManager certmanager.CertificateManager) *ConsoleREST {
	return &ConsoleREST{
		vmLister:         vmLister,
		kubevirt:         kubevirt,
		proxyCertManager: proxyCertManager,
	}
}

// New implements rest.Storage interface
func (r ConsoleREST) New() runtime.Object {
	return &subresources.VirtualMachineConsole{}
}

// Destroy implements rest.Storage interface
func (r ConsoleREST) Destroy() {
}

func (r ConsoleREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	consoleOpts, ok := opts.(*subresources.VirtualMachineConsole)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	location, transport, err := ConsoleLocation(ctx, r.vmLister, name, consoleOpts, r.kubevirt, r.proxyCertManager)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, true, responder, r.kubevirt.ServiceAccount)
	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (r ConsoleREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachineConsole{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r ConsoleREST) ConnectMethods() []string {
	return upgradeableMethods
}

func ConsoleLocation(
	ctx context.Context,
	getter cache.GenericLister,
	name string,
	opts *subresources.VirtualMachineConsole,
	kubevirt KubevirtApiServerConfig,
	proxyCertManager certmanager.CertificateManager,
) (*url.URL, *http.Transport, error) {
	return streamLocation(ctx, getter, name, opts, "console", kubevirt, proxyCertManager)
}
