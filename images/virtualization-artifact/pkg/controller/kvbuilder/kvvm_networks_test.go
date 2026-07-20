/*
Copyright 2026 Flant JSC

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

package kvbuilder

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("setNetworksAnnotation", func() {
	newKVVM := func() *KVVM {
		return NewEmptyKVVM(namespacedName("test-vm", "test-ns"), KVVMOptions{})
	}

	mainOnly := network.InterfaceSpecList{
		{Type: v1alpha2.NetworksTypeMain, InterfaceName: network.NameDefaultInterface},
	}

	It("omits the annotations for a VM without additional networks", func() {
		kvvm := newKVVM()
		Expect(setNetworksAnnotation(kvvm, mainOnly)).To(Succeed())

		anno := kvvm.Resource.Spec.Template.ObjectMeta.GetAnnotations()
		Expect(anno).NotTo(HaveKey(annotations.AnnNetworksSpec))
		Expect(anno).NotTo(HaveKey(annotations.AnnTapProvisionByDVPSupported))
	})

	It("clears a stale empty networks-spec annotation", func() {
		kvvm := newKVVM()
		kvvm.SetKVVMIAnnotation(annotations.AnnNetworksSpec, "[]")
		kvvm.SetKVVMIAnnotation(annotations.AnnTapProvisionByDVPSupported, "true")

		Expect(setNetworksAnnotation(kvvm, mainOnly)).To(Succeed())

		anno := kvvm.Resource.Spec.Template.ObjectMeta.GetAnnotations()
		Expect(anno).NotTo(HaveKey(annotations.AnnNetworksSpec))
		Expect(anno).NotTo(HaveKey(annotations.AnnTapProvisionByDVPSupported))
	})

	It("sets the annotations when there is an additional network", func() {
		kvvm := newKVVM()
		Expect(setNetworksAnnotation(kvvm, network.InterfaceSpecList{
			{Type: v1alpha2.NetworksTypeMain, InterfaceName: network.NameDefaultInterface},
			{Type: v1alpha2.NetworksTypeClusterNetwork, Name: "cnet", InterfaceName: "veth_cn12345678", UID: 64535, GID: 64535},
		})).To(Succeed())

		anno := kvvm.Resource.Spec.Template.ObjectMeta.GetAnnotations()
		Expect(anno[annotations.AnnNetworksSpec]).NotTo(BeEmpty())
		Expect(anno[annotations.AnnNetworksSpec]).NotTo(Equal("[]"))
		Expect(anno).To(HaveKeyWithValue(annotations.AnnTapProvisionByDVPSupported, "true"))
	})
})
