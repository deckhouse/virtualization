package kubecli

import (
	coreinstall "github.com/deckhouse/virtualization-controller/api/core/install"
	subinstall "github.com/deckhouse/virtualization-controller/api/subresources/install"
	"io"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	"net"

	"github.com/deckhouse/virtualization-controller/api/client/generated/clientset/versioned"
	virtualizationv1alpha2 "github.com/deckhouse/virtualization-controller/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization-controller/api/subresources/v1alpha2"
)

var (
	Scheme = runtime.NewScheme()
	Codecs = serializer.NewCodecFactory(Scheme)
)

func init() {
	subinstall.Install(Scheme)
	coreinstall.Install(Scheme)
}

type Client interface {
	VirtualMachines(namespace string) VirtualMachineInterface
}
type StreamOptions struct {
	In  io.Reader
	Out io.Writer
}

type StreamInterface interface {
	Stream(options StreamOptions) error
	AsConn() net.Conn
}

type VirtualMachineInterface interface {
	virtualizationv1alpha2.VirtualMachineInterface
	SerialConsole(name string, options *SerialConsoleOptions) (StreamInterface, error)
	VNC(name string) (StreamInterface, error)
	PortForward(name string, opts v1alpha2.VirtualMachinePortForward) (StreamInterface, error)
}

type client struct {
	config      *rest.Config
	shallowCopy *rest.Config
	restClient  *rest.RESTClient
	virtClient  *versioned.Clientset
}

func (c client) VirtualMachines(namespace string) VirtualMachineInterface {
	return &vm{
		VirtualMachineInterface: c.virtClient.VirtualizationV1alpha2().VirtualMachines(namespace),
		restClient:              c.restClient,
		config:                  c.config,
		namespace:               namespace,
		resource:                "virtualmachines",
	}
}
