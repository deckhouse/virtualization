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
	"github.com/deckhouse/virtualization-controller/pkg/apiserver/apis/operations"
	"github.com/deckhouse/virtualization-controller/pkg/apiserver/apis/operations/install"
)

type KubevirtApiServerConfig struct {
	Endpoint  string
	CertsPath string
}

var (
	Scheme = runtime.NewScheme()
	Codecs = serializer.NewCodecFactory(Scheme)
)

func init() {
	install.Install(Scheme)
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})
}

func Build(vm rest.Storage) genericapiserver.APIGroupInfo {
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(operations.GroupName, Scheme, metav1.ParameterCodec, Codecs)
	resources := map[string]rest.Storage{
		"virtualmachines": vm,
	}
	apiGroupInfo.VersionedResourcesStorageMap[virtv2.SchemeGroupVersion.Version] = resources
	return apiGroupInfo
}

func Install(vmLister cache.GenericLister, server *genericapiserver.GenericAPIServer, kubevirt KubevirtApiServerConfig) error {
	vmConsole := newConsole(operations.Resource("virtualmachines"), vmLister, kubevirt)
	info := Build(vmConsole)
	return server.InstallAPIGroup(&info)
}
