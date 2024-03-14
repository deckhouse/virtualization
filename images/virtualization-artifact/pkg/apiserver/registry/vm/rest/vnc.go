package rest

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/virtualization-controller/api/subresources"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
)

type VNCREST struct {
	vmLister         cache.GenericLister
	proxyCertManager certmanager.CertificateManager
	kubevirt         KubevirtApiServerConfig
}

var (
	_ rest.Storage   = &VNCREST{}
	_ rest.Connecter = &VNCREST{}
)

func NewVNCREST(vmLister cache.GenericLister, kubevirt KubevirtApiServerConfig, proxyCertManager certmanager.CertificateManager) *VNCREST {
	return &VNCREST{
		vmLister:         vmLister,
		kubevirt:         kubevirt,
		proxyCertManager: proxyCertManager,
	}
}

// New implements rest.Storage interface
func (r VNCREST) New() runtime.Object {
	return &subresources.VirtualMachineVNC{}
}

// Destroy implements rest.Storage interface
func (r VNCREST) Destroy() {
}

func (r VNCREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	vncOpts, ok := opts.(*subresources.VirtualMachineVNC)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	location, transport, err := VNCLocation(ctx, r.vmLister, name, vncOpts, r.kubevirt, r.proxyCertManager)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, false, true, responder, r.kubevirt.ServiceAccount)
	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (r VNCREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachineVNC{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r VNCREST) ConnectMethods() []string {
	return upgradeableMethods
}

func VNCLocation(
	ctx context.Context,
	getter cache.GenericLister,
	name string,
	opts *subresources.VirtualMachineVNC,
	kubevirt KubevirtApiServerConfig,
	proxyCertManager certmanager.CertificateManager,
) (*url.URL, *http.Transport, error) {
	return streamLocation(ctx, getter, name, opts, "vnc", kubevirt, proxyCertManager)
}
