package rest

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/tools/cache"

	virtv2 "github.com/deckhouse/virtualization-controller/api/core/v1alpha2"
	"github.com/deckhouse/virtualization-controller/api/subresources"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
)

const (
	userHeader       = "X-Remote-User"
	groupHeader      = "X-Remote-Group"
	kubevirtPathTmpl = "/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachineinstances/%s/%s"
)

var upgradeableMethods = []string{http.MethodGet, http.MethodPost}

func streamLocation(
	ctx context.Context,
	getter cache.GenericLister,
	name string,
	opts runtime.Object,
	streamPath string,
	kubevirt KubevirtApiServerConfig,
	proxyCertManager certmanager.CertificateManager,
) (*url.URL, *http.Transport, error) {
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
		Path:     fmt.Sprintf(kubevirtPathTmpl, vm.Namespace, name, streamPath),
		RawQuery: params.Encode(),
	}
	ca, err := os.ReadFile(kubevirt.CaBundlePath)
	if err != nil {
		return nil, nil, err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(ca)

	cert := proxyCertManager.Current()
	if cert == nil {
		return nil, nil, fmt.Errorf("proxy certificate not found")
	}
	tlsConfig := &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{*cert},
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
		return nil, fmt.Errorf("unexpected object type: %#v", vm)
	}
	return vm, nil
}

// TODO: This may be useful in the future
func streamParams(_ url.Values, opts runtime.Object) error {
	switch opts := opts.(type) {
	case *subresources.VirtualMachineConsole:
		return nil
	case *subresources.VirtualMachineVNC:
		return nil
	case *subresources.VirtualMachinePortForward:
		return nil
	default:
		return fmt.Errorf("unknown object for streaming: %v", opts)
	}
}

func newThrottledUpgradeAwareProxyHandler(
	location *url.URL,
	transport *http.Transport,
	wrapTransport, upgradeRequired bool,
	responder rest.Responder,
	sa types.NamespacedName,
) http.Handler {
	var handler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		r.Header.Add(userHeader, fmt.Sprintf("system:serviceaccount:%s:%s", sa.Namespace, sa.Name))
		r.Header.Add(groupHeader, "system:serviceaccounts")
		proxyHandler := proxy.NewUpgradeAwareHandler(location, transport, wrapTransport, upgradeRequired, proxy.NewErrorResponder(responder))
		proxyHandler.ServeHTTP(w, r)
	}
	return handler
}
