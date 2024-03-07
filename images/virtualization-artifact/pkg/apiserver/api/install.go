package api

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/tools/cache"

	rest2 "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
	"github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/storage"

	"github.com/deckhouse/virtualization-controller/api/subresources"
	"github.com/deckhouse/virtualization-controller/api/subresources/install"
	"github.com/deckhouse/virtualization-controller/api/subresources/v1alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certManager"
)

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

func Build(store *storage.VirtualMachineStorage) genericapiserver.APIGroupInfo {
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(subresources.GroupName, Scheme, metav1.ParameterCodec, Codecs)
	resources := map[string]rest.Storage{
		"virtualmachines":             store,
		"virtualmachines/console":     store.ConsoleREST(),
		"virtualmachines/vnc":         store.VncREST(),
		"virtualmachines/portforward": store.PortForwardREST(),
	}
	apiGroupInfo.VersionedResourcesStorageMap[v1alpha1.SchemeGroupVersion.Version] = resources
	return apiGroupInfo
}

func Install(
	vmLister cache.GenericLister,
	server *genericapiserver.GenericAPIServer,
	kubevirt rest2.KubevirtApiServerConfig,
	proxyCertManager certManager.CertificateManager,
) error {
	vmStorage := storage.NewStorage(subresources.Resource("virtualmachines"), vmLister, kubevirt, proxyCertManager)
	info := Build(vmStorage)
	return server.InstallAPIGroup(&info)
}
