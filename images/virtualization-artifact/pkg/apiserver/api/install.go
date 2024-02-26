package api

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/virtualization-controller/api/operations"
	"github.com/deckhouse/virtualization-controller/api/operations/install"
	"github.com/deckhouse/virtualization-controller/api/operations/v1alpha1"
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

	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	Scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
}

func Build(vm rest.Storage) genericapiserver.APIGroupInfo {
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(operations.GroupName, Scheme, metav1.ParameterCodec, Codecs)
	resources := map[string]rest.Storage{
		"virtualmachines": vm,
	}
	apiGroupInfo.VersionedResourcesStorageMap[v1alpha1.SchemeGroupVersion.Version] = resources
	return apiGroupInfo
}

func Install(vmLister cache.GenericLister, server *genericapiserver.GenericAPIServer, kubevirt KubevirtApiServerConfig) error {
	vmConsole := newConsole(operations.Resource("virtualmachines"), vmLister, kubevirt)
	info := Build(vmConsole)
	return server.InstallAPIGroup(&info)
}
