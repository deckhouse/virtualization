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

package testutil

import (
	virtualizationfake "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/fake"
	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

type FakeClient struct {
	*k8sfake.Clientset
	virtClient *virtualizationfake.Clientset
}

func NewFakeClient() *FakeClient {
	return &FakeClient{
		Clientset:  k8sfake.NewSimpleClientset(),
		virtClient: virtualizationfake.NewSimpleClientset(),
	}
}

func (f *FakeClient) ClusterVirtualImages() virtualizationv1alpha2.ClusterVirtualImageInterface {
	return f.virtClient.VirtualizationV1alpha2().ClusterVirtualImages()
}

func (f *FakeClient) VirtualMachines(namespace string) virtualizationv1alpha2.VirtualMachineInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachines(namespace)
}

func (f *FakeClient) VirtualImages(namespace string) virtualizationv1alpha2.VirtualImageInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualImages(namespace)
}

func (f *FakeClient) VirtualDisks(namespace string) virtualizationv1alpha2.VirtualDiskInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualDisks(namespace)
}

func (f *FakeClient) VirtualMachineBlockDeviceAttachments(namespace string) virtualizationv1alpha2.VirtualMachineBlockDeviceAttachmentInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachineBlockDeviceAttachments(namespace)
}

func (f *FakeClient) VirtualMachineIPAddresses(namespace string) virtualizationv1alpha2.VirtualMachineIPAddressInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachineIPAddresses(namespace)
}

func (f *FakeClient) VirtualMachineIPAddressLeases() virtualizationv1alpha2.VirtualMachineIPAddressLeaseInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachineIPAddressLeases()
}

func (f *FakeClient) VirtualMachineOperations(namespace string) virtualizationv1alpha2.VirtualMachineOperationInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachineOperations(namespace)
}

func (f *FakeClient) VirtualMachineClasses() virtualizationv1alpha2.VirtualMachineClassInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachineClasses()
}

func (f *FakeClient) VirtualMachineMACAddresses(namespace string) virtualizationv1alpha2.VirtualMachineMACAddressInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachineMACAddresses(namespace)
}

func (f *FakeClient) VirtualMachineMACAddressLeases() virtualizationv1alpha2.VirtualMachineMACAddressLeaseInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachineMACAddressLeases()
}

func (f *FakeClient) NodeUSBDevices() virtualizationv1alpha2.NodeUSBDeviceInterface {
	return f.virtClient.VirtualizationV1alpha2().NodeUSBDevices()
}

func (f *FakeClient) USBDevices(namespace string) virtualizationv1alpha2.USBDeviceInterface {
	return f.virtClient.VirtualizationV1alpha2().USBDevices(namespace)
}
