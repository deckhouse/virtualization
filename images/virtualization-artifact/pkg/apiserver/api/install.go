package api

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/tools/cache"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v1alpha2"
)

type KubevirtApiServerConfig struct {
	Endpoint  string
	CertsPath string
}

var (
	// Scheme contains the types needed by the resource metrics API.
	Scheme = runtime.NewScheme()
	// Codecs is a codec factory for serving the resource metrics API.
	Codecs = serializer.NewCodecFactory(Scheme)
)

func init() {
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: virtv2.APIVersion})
}

func Build(vm rest.Storage) genericapiserver.APIGroupInfo {
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(virtv2.APIGroup, Scheme, metav1.ParameterCodec, Codecs)
	resources := map[string]rest.Storage{
		virtv2.VMResource: vm,
	}
	apiGroupInfo.VersionedResourcesStorageMap[virtv2.SchemeGroupVersion.Version] = resources
	return apiGroupInfo
}

func Install(vmLister cache.GenericLister, server *genericapiserver.GenericAPIServer, kubevirt KubevirtApiServerConfig) error {
	vmConsole := newConsole(virtv2.Resource(virtv2.VirtualMachineConsoleResource), vmLister, kubevirt)
	info := Build(vmConsole)
	return server.InstallAPIGroup(&info)
}
