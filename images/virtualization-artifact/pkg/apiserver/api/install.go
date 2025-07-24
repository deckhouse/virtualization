/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"

	vmrest "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
	"github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/storage"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	versionedv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
	"github.com/deckhouse/virtualization/api/subresources/install"
	"github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

var (
	Scheme         = runtime.NewScheme()
	Codecs         = serializer.NewCodecFactory(Scheme)
	ParameterCodec = runtime.NewParameterCodec(Scheme)
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
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(subresources.GroupName, Scheme, ParameterCodec, Codecs)
	resources := map[string]rest.Storage{
		"virtualmachines":                  store,
		"virtualmachines/console":          store.ConsoleREST(),
		"virtualmachines/usbredir":         store.UsbRedirREST(),
		"virtualmachines/vnc":              store.VncREST(),
		"virtualmachines/portforward":      store.PortForwardREST(),
		"virtualmachines/addvolume":        store.AddVolumeREST(),
		"virtualmachines/removevolume":     store.RemoveVolumeREST(),
		"virtualmachines/freeze":           store.FreezeREST(),
		"virtualmachines/unfreeze":         store.UnfreezeREST(),
		"virtualmachines/cancelevacuation": store.CancelEvacuationREST(),
	}
	apiGroupInfo.VersionedResourcesStorageMap[v1alpha2.SchemeGroupVersion.Version] = resources
	return apiGroupInfo
}

func Install(
	vmLister virtlisters.VirtualMachineLister,
	server *genericapiserver.GenericAPIServer,
	kubevirt vmrest.KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
	crd *apiextensionsv1.CustomResourceDefinition,
	vmClient versionedv1alpha2.VirtualMachinesGetter,
) error {
	vmStorage := storage.NewStorage(subresources.Resource("virtualmachines"),
		vmLister,
		kubevirt,
		proxyCertManager,
		crd,
		vmClient,
	)
	info := Build(vmStorage)
	return server.InstallAPIGroup(&info)
}
