/*
Copyright 2022 Flant JSC

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
// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeVirtualizationV1alpha2 struct {
	*testing.Fake
}

func (c *FakeVirtualizationV1alpha2) ClusterVirtualImages() v1alpha2.ClusterVirtualImageInterface {
	return &FakeClusterVirtualImages{c}
}

func (c *FakeVirtualizationV1alpha2) VirtualDisks(namespace string) v1alpha2.VirtualDiskInterface {
	return &FakeVirtualDisks{c, namespace}
}

func (c *FakeVirtualizationV1alpha2) VirtualDiskSnapshots(namespace string) v1alpha2.VirtualDiskSnapshotInterface {
	return &FakeVirtualDiskSnapshots{c, namespace}
}

func (c *FakeVirtualizationV1alpha2) VirtualImages(namespace string) v1alpha2.VirtualImageInterface {
	return &FakeVirtualImages{c, namespace}
}

func (c *FakeVirtualizationV1alpha2) VirtualMachines(namespace string) v1alpha2.VirtualMachineInterface {
	return &FakeVirtualMachines{c, namespace}
}

func (c *FakeVirtualizationV1alpha2) VirtualMachineBlockDeviceAttachments(namespace string) v1alpha2.VirtualMachineBlockDeviceAttachmentInterface {
	return &FakeVirtualMachineBlockDeviceAttachments{c, namespace}
}

func (c *FakeVirtualizationV1alpha2) VirtualMachineClasses() v1alpha2.VirtualMachineClassInterface {
	return &FakeVirtualMachineClasses{c}
}

func (c *FakeVirtualizationV1alpha2) VirtualMachineIPAddresses(namespace string) v1alpha2.VirtualMachineIPAddressInterface {
	return &FakeVirtualMachineIPAddresses{c, namespace}
}

func (c *FakeVirtualizationV1alpha2) VirtualMachineIPAddressLeases() v1alpha2.VirtualMachineIPAddressLeaseInterface {
	return &FakeVirtualMachineIPAddressLeases{c}
}

func (c *FakeVirtualizationV1alpha2) VirtualMachineOperations(namespace string) v1alpha2.VirtualMachineOperationInterface {
	return &FakeVirtualMachineOperations{c, namespace}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeVirtualizationV1alpha2) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
