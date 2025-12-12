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
	virtualizationv1alpha2.VirtualizationV1alpha2Interface
}
type client struct {
	kubernetes.Interface
	virtualizationv1alpha2.VirtualizationV1alpha2Interface
	config      *rest.Config
	shallowCopy *rest.Config
	restClient  *rest.RESTClient
}

func (c client) VirtualMachines(namespace string) virtualizationv1alpha2.VirtualMachineInterface {
	return &vm{
		VirtualMachineInterface: c.VirtualizationV1alpha2Interface.VirtualMachines(namespace),
		restClient:              c.restClient,
		config:                  c.config,
		namespace:               namespace,
		resource:                "virtualmachines",
	}
}
