package v1alpha2

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/deckhouse/virtualization-controller/pkg/apiserver"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/kubernetes"
	"net/http"
	"net/url"
	"os"
	"path"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource"
	"sigs.k8s.io/apiserver-runtime/pkg/util/loopback"
)

var _ resource.ConnectorSubResource = &VirtualMachineConsole{}

// VirtualMachineConsole is a sub-resource for connecting to VirtualMachine using the console.
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VirtualMachineConsole struct {
	metav1.TypeMeta `json:",inline"`
}

func (c *VirtualMachineConsole) Destroy() {}

func (c *VirtualMachineConsole) Connect(ctx context.Context, id string, options runtime.Object, r rest.Responder) (http.Handler, error) {
	_, ok := options.(*VirtualMachineConsole)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", options)
	}
	conf, err := apiserver.GetConf()
	if err != nil {
		return nil, err
	}
	clientConfig := loopback.GetLoopbackClientConfig()
	client, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, err
	}
	ns, _ := request.NamespaceFrom(ctx)
	pods, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("vm.kubevirt.io/name=%s", id),
	})
	if err != nil {
		return nil, err
	}
	if pods == nil || len(pods.Items) == 0 {
		return nil, fmt.Errorf("pod not found")
	}
	if pods.Items[0].Status.Phase != corev1.PodRunning {
		return nil, fmt.Errorf("pod is not running")
	}

	location := &url.URL{
		Scheme: "https",
		Host:   conf.KVApiServerEndpoint,
		Path:   fmt.Sprintf("/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachine/%s/console", ns, id),
	}
	ca, err := os.ReadFile(path.Join(conf.KVApiServerCertsPath, "ca.crt"))
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(ca)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
		RootCAs:            caCertPool,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return newUpgradeAwareProxyHandler(location, transport, false, true, true, responder), nil
}

func (c *VirtualMachineConsole) NewConnectOptions() (runtime.Object, bool, string) {
	return &VirtualMachineConsole{}, false, ""
}

func (c *VirtualMachineConsole) ConnectMethods() []string {
	return []string{http.MethodGet}
}

func (c *VirtualMachineConsole) SubResourceName() string {
	return "console"
}
func (c *VirtualMachineConsole) New() runtime.Object {
	return &VirtualMachineConsole{}
}

func newUpgradeAwareProxyHandler(location *url.URL, transport http.RoundTripper, wrapTransport, upgradeRequired, interceptRedirects bool, responder rest.Responder) *proxy.UpgradeAwareHandler {
	handler := proxy.NewUpgradeAwareHandler(location, transport, wrapTransport, upgradeRequired, proxy.NewErrorResponder(responder))
	handler.MaxBytesPerSec = 0
	return handler
}
