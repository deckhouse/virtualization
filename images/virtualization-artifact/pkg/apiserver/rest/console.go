package rest

import (
	"context"
	"fmt"
	virtv2 "github.com/deckhouse/virtualization-controller/api/core/v1alpha2"
	"github.com/deckhouse/virtualization-controller/api/operations"
	"github.com/deckhouse/virtualization-controller/pkg/apiserver/storage"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certManager"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/tools/cache"
	"net/http"
	"net/url"
)

const consoleTemplateURI = "wss://%s/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachine/%s/%s"

type ConsoleREST struct {
	groupResource    schema.GroupResource
	vmLister         cache.GenericLister
	proxyCertManager certManager.CertificateManager
	kubevirt         KubevirtApiServerConfig
}

type KubevirtApiServerConfig struct {
	Endpoint     string
	CaBundlePath string
}

var (
	_ rest.Storage   = &ConsoleREST{}
	_ rest.Connecter = &ConsoleREST{}
)

func NewConsoleREST(groupResource schema.GroupResource, vmLister cache.GenericLister, kubevirt KubevirtApiServerConfig, proxyCertManager certManager.CertificateManager) *ConsoleREST {
	return &ConsoleREST{
		groupResource:    groupResource,
		vmLister:         vmLister,
		kubevirt:         kubevirt,
		proxyCertManager: proxyCertManager,
	}
}

// New implements rest.Storage interface
func (r ConsoleREST) New() runtime.Object {
	return &operations.VirtualMachineConsole{}
}

// Destroy implements rest.Storage interface
func (r ConsoleREST) Destroy() {
}

func (r ConsoleREST) getFetcherVirtualMachine(name, namespace string) (*virtv2.VirtualMachine, *errors.StatusError) {
	return storage.FetchVirtualMachine(r.vmLister, name, namespace)
}
func (r ConsoleREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	consoleOpts, ok := opts.(*operations.VirtualMachineConsole)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	location, transport, err := ConsoleLocation(ctx, r.vmLister, name, consoleOpts, r.kubevirt, r.proxyCertManager)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, false, true, responder)
	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (r ConsoleREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &operations.VirtualMachineConsole{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r ConsoleREST) ConnectMethods() []string {
	return upgradeableMethods
}

var upgradeableMethods = []string{http.MethodGet, http.MethodPost}

func ConsoleLocation(
	ctx context.Context,
	getter cache.GenericLister,
	name string,
	opts *operations.VirtualMachineConsole,
	kubevirt KubevirtApiServerConfig,
	proxyCertManager certManager.CertificateManager,
) (*url.URL, *http.Transport, error) {
	return streamLocation(ctx, getter, name, opts, "console", kubevirt, proxyCertManager)
}
