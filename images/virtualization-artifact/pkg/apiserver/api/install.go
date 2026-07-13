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
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"

	vmrest "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
	"github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/storage"
	vmpoolstorage "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vmpool/storage"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	virtclient "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
	"github.com/deckhouse/virtualization/api/subresources/install"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

var (
	Scheme         = runtime.NewScheme()
	Codecs         = serializer.NewCodecFactory(Scheme)
	ParameterCodec = runtime.NewParameterCodec(Scheme)
)

func init() {
	install.Install(Scheme)
	// autoscaling/v1 Scale is served by the virtualmachines/scale subresource so
	// the stock VPA recommender can discover a VM's virt-launcher pod. It lives in a
	// different group than the API being served, so the Scheme must know how to
	// encode it.
	utilruntime.Must(autoscalingv1.AddToScheme(Scheme))
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

func Build(store *storage.VirtualMachineStorage, poolStorage *vmpoolstorage.VirtualMachinePoolStorage) genericapiserver.APIGroupInfo {
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(subresources.GroupName, Scheme, ParameterCodec, Codecs)
	resourcesV1alpha2 := map[string]rest.Storage{
		"virtualmachines":                     store,
		"virtualmachines/console":             store.ConsoleREST(),
		"virtualmachines/vnc":                 store.VncREST(),
		"virtualmachines/portforward":         store.PortForwardREST(),
		"virtualmachines/addvolume":           store.AddVolumeREST(),
		"virtualmachines/removevolume":        store.RemoveVolumeREST(),
		"virtualmachines/freeze":              store.FreezeREST(),
		"virtualmachines/unfreeze":            store.UnfreezeREST(),
		"virtualmachines/cancelevacuation":    store.CancelEvacuationREST(),
		"virtualmachines/addresourceclaim":    store.AddResourceClaimREST(),
		"virtualmachines/removeresourceclaim": store.RemoveResourceClaimREST(),
		"virtualmachines/scale":               store.ScaleREST(),
	}
	// Enterprise-only resources (e.g. virtualmachinepools/scaledownwith) are added
	// only in paid editions; poolStorage is nil in CE, leaving the map untouched.
	installEnterpriseResources(resourcesV1alpha2, poolStorage)
	apiGroupInfo.VersionedResourcesStorageMap[subv1alpha2.SchemeGroupVersion.Version] = resourcesV1alpha2
	return apiGroupInfo
}

func Install(
	vmLister virtlisters.VirtualMachineLister,
	server *genericapiserver.GenericAPIServer,
	kubevirt vmrest.KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
	virtCli virtclient.Interface,
) error {
	vmStorage := storage.NewStorage(
		vmLister,
		kubevirt,
		proxyCertManager,
	)
	// Enterprise (EE/SE+) subresources are constructed here and injected, the same
	// way vmStorage is. They are registered unconditionally: the apiserver process
	// has neither the runtime feature gate (not wired in here) nor the compiled-in
	// edition (the virtualization-api binary is built without an edition tag), so
	// it cannot gate them itself. Availability is enforced upstream — the pool CRD
	// is installed only when the feature gate is on, and the controller self-gates;
	// with no CRD the endpoint simply resolves to NotFound.
	poolStorage := vmpoolstorage.NewStorage(virtCli)
	info := Build(vmStorage, poolStorage)
	return server.InstallAPIGroup(&info)
}
