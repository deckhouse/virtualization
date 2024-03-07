package kubecli

import (
	"io"
	"net"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	"github.com/deckhouse/virtualization-controller/api/client/generated/clientset/versioned"
	virtualizationv1alpha2 "github.com/deckhouse/virtualization-controller/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization-controller/api/core/v1alpha2"
	"github.com/deckhouse/virtualization-controller/api/subresources/v1alpha1"
)

var (
	SchemeBuilder  runtime.SchemeBuilder
	Scheme         *runtime.Scheme
	Codecs         serializer.CodecFactory
	ParameterCodec runtime.ParameterCodec
)

func init() {
	SchemeBuilder = v1alpha2.SchemeBuilder
	Scheme = runtime.NewScheme()
	AddToScheme := SchemeBuilder.AddToScheme
	Codecs = serializer.NewCodecFactory(Scheme)
	ParameterCodec = runtime.NewParameterCodec(Scheme)
	AddToScheme(Scheme)
	AddToScheme(scheme.Scheme)
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
	PortForward(name string, opts v1alpha1.VirtualMachinePortForward) (StreamInterface, error)
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
