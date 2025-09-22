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

package kubeclient

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/deckhouse/virtualization/api/client/generated/clientset/versioned"
	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	coreinstall "github.com/deckhouse/virtualization/api/core/install"
	subinstall "github.com/deckhouse/virtualization/api/subresources/install"
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
	kubernetes.Interface
	ClusterVirtualImages() virtualizationv1alpha2.ClusterVirtualImageInterface
	VirtualMachines(namespace string) virtualizationv1alpha2.VirtualMachineInterface
	VirtualImages(namespace string) virtualizationv1alpha2.VirtualImageInterface
	VirtualDisks(namespace string) virtualizationv1alpha2.VirtualDiskInterface
	VirtualMachineBlockDeviceAttachments(namespace string) virtualizationv1alpha2.VirtualMachineBlockDeviceAttachmentInterface
	VirtualMachineIPAddresses(namespace string) virtualizationv1alpha2.VirtualMachineIPAddressInterface
	VirtualMachineIPAddressLeases() virtualizationv1alpha2.VirtualMachineIPAddressLeaseInterface
	VirtualMachineOperations(namespace string) virtualizationv1alpha2.VirtualMachineOperationInterface
	VirtualMachineClasses() virtualizationv1alpha2.VirtualMachineClassInterface
	VirtualMachineMACAddresses(namespace string) virtualizationv1alpha2.VirtualMachineMACAddressInterface
	VirtualMachineMACAddressLeases() virtualizationv1alpha2.VirtualMachineMACAddressLeaseInterface
}
type client struct {
	kubernetes.Interface
	config      *rest.Config
	shallowCopy *rest.Config
	restClient  *rest.RESTClient
	virtClient  *versioned.Clientset
}

func (c client) VirtualMachines(namespace string) virtualizationv1alpha2.VirtualMachineInterface {
	return &vm{
		VirtualMachineInterface: c.virtClient.VirtualizationV1alpha2().VirtualMachines(namespace),
		restClient:              c.restClient,
		config:                  c.config,
		namespace:               namespace,
		resource:                "apivirtualmachines",
	}
}

func (c client) ClusterVirtualImages() virtualizationv1alpha2.ClusterVirtualImageInterface {
	return c.virtClient.VirtualizationV1alpha2().ClusterVirtualImages()
}

func (c client) VirtualImages(namespace string) virtualizationv1alpha2.VirtualImageInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualImages(namespace)
}

func (c client) VirtualDisks(namespace string) virtualizationv1alpha2.VirtualDiskInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualDisks(namespace)
}

func (c client) VirtualMachineBlockDeviceAttachments(namespace string) virtualizationv1alpha2.VirtualMachineBlockDeviceAttachmentInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualMachineBlockDeviceAttachments(namespace)
}

func (c client) VirtualMachineIPAddresses(namespace string) virtualizationv1alpha2.VirtualMachineIPAddressInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualMachineIPAddresses(namespace)
}

func (c client) VirtualMachineIPAddressLeases() virtualizationv1alpha2.VirtualMachineIPAddressLeaseInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualMachineIPAddressLeases()
}

func (c client) VirtualMachineOperations(namespace string) virtualizationv1alpha2.VirtualMachineOperationInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualMachineOperations(namespace)
}

func (c client) VirtualMachineClasses() virtualizationv1alpha2.VirtualMachineClassInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualMachineClasses()
}

func (c client) VirtualMachineMACAddresses(namespace string) virtualizationv1alpha2.VirtualMachineMACAddressInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualMachineMACAddresses(namespace)
}

func (c client) VirtualMachineMACAddressLeases() virtualizationv1alpha2.VirtualMachineMACAddressLeaseInterface {
	return c.virtClient.VirtualizationV1alpha2().VirtualMachineMACAddressLeases()
}
