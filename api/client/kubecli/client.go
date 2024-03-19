package kubecli

import (
	"io"
	"net"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"

	"github.com/deckhouse/virtualization/api/client/generated/clientset/versioned"
	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	coreinstall "github.com/deckhouse/virtualization/api/core/install"
	subinstall "github.com/deckhouse/virtualization/api/subresources/install"
	"github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

var (
	Scheme = runtime.NewScheme()
	Codecs = serializer.NewCodecFactory(Scheme)
)

func init() {
	coreinstall.Install(Scheme)
	subinstall.Install(Scheme)
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})

	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	Scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
}

type Client interface {
	ClusterVirtualMachineImages() virtualizationv1alpha2.ClusterVirtualMachineImageInterface
	VirtualMachines(namespace string) VirtualMachineInterface
	VirtualMachineImages(namespace string) virtualizationv1alpha2.VirtualMachineImageInterface
	VirtualMachineDisks(namespace string) virtualizationv1alpha2.VirtualMachineDiskInterface
	VirtualMachineBlockDeviceAttachments(namespace string) virtualizationv1alpha2.VirtualMachineBlockDeviceAttachmentInterface
	VirtualMachineIPAddressClaims(namespace string) virtualizationv1alpha2.VirtualMachineIPAddressClaimInterface
	VirtualMachineIPAddressLeases(namespace string) virtualizationv1alpha2.VirtualMachineIPAddressLeaseInterface
	VirtualMachineOperations(namespace string) virtualizationv1alpha2.VirtualMachineOperationInterface
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

func (c client) ClusterVirtualMachineImages() virtualizationv1alpha2.ClusterVirtualMachineImageInterface {
	return c.virtClient.VirtualizationV1alpha2().ClusterVirtualMachineImages()
}

func (c client) VirtualMachineImages(namespace string) virtualizationv1alpha2.VirtualMachineImageInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualMachineImages(namespace)
}

func (c client) VirtualMachineDisks(namespace string) virtualizationv1alpha2.VirtualMachineDiskInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualMachineDisks(namespace)
}

func (c client) VirtualMachineBlockDeviceAttachments(namespace string) virtualizationv1alpha2.VirtualMachineBlockDeviceAttachmentInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualMachineBlockDeviceAttachments(namespace)
}

func (c client) VirtualMachineIPAddressClaims(namespace string) virtualizationv1alpha2.VirtualMachineIPAddressClaimInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualMachineIPAddressClaims(namespace)
}

func (c client) VirtualMachineIPAddressLeases(namespace string) virtualizationv1alpha2.VirtualMachineIPAddressLeaseInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualMachineIPAddressLeases(namespace)
}

func (c client) VirtualMachineOperations(namespace string) virtualizationv1alpha2.VirtualMachineOperationInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualMachineOperations(namespace)
}
