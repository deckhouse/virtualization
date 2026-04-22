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

package vnc

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	virtualizationfake "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/fake"
	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
)

type fakeClient struct {
	*k8sfake.Clientset
	virtClient *virtualizationfake.Clientset
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		Clientset:  k8sfake.NewSimpleClientset(),
		virtClient: virtualizationfake.NewSimpleClientset(),
	}
}

func (f *fakeClient) ClusterVirtualImages() virtualizationv1alpha2.ClusterVirtualImageInterface {
	return f.virtClient.VirtualizationV1alpha2().ClusterVirtualImages()
}

func (f *fakeClient) VirtualMachines(namespace string) virtualizationv1alpha2.VirtualMachineInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachines(namespace)
}

func (f *fakeClient) VirtualImages(namespace string) virtualizationv1alpha2.VirtualImageInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualImages(namespace)
}

func (f *fakeClient) VirtualDisks(namespace string) virtualizationv1alpha2.VirtualDiskInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualDisks(namespace)
}

func (f *fakeClient) VirtualMachineBlockDeviceAttachments(namespace string) virtualizationv1alpha2.VirtualMachineBlockDeviceAttachmentInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachineBlockDeviceAttachments(namespace)
}

func (f *fakeClient) VirtualMachineIPAddresses(namespace string) virtualizationv1alpha2.VirtualMachineIPAddressInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachineIPAddresses(namespace)
}

func (f *fakeClient) VirtualMachineIPAddressLeases() virtualizationv1alpha2.VirtualMachineIPAddressLeaseInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachineIPAddressLeases()
}

func (f *fakeClient) VirtualMachineOperations(namespace string) virtualizationv1alpha2.VirtualMachineOperationInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachineOperations(namespace)
}

func (f *fakeClient) VirtualMachineClasses() virtualizationv1alpha2.VirtualMachineClassInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachineClasses()
}

func (f *fakeClient) VirtualMachineMACAddresses(namespace string) virtualizationv1alpha2.VirtualMachineMACAddressInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachineMACAddresses(namespace)
}

func (f *fakeClient) VirtualMachineMACAddressLeases() virtualizationv1alpha2.VirtualMachineMACAddressLeaseInterface {
	return f.virtClient.VirtualizationV1alpha2().VirtualMachineMACAddressLeases()
}

func (f *fakeClient) NodeUSBDevices() virtualizationv1alpha2.NodeUSBDeviceInterface {
	return f.virtClient.VirtualizationV1alpha2().NodeUSBDevices()
}

func (f *fakeClient) USBDevices(namespace string) virtualizationv1alpha2.USBDeviceInterface {
	return f.virtClient.VirtualizationV1alpha2().USBDevices(namespace)
}

func TestVNC(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VNC Command Suite")
}

var _ = Describe("VNC", func() {
	var (
		oldProxyOnly                     bool
		oldCustomPort                    int
		oldListenAddress                 string
		oldClientAndNamespaceFromContext func(context.Context) (kubeclient.Client, string, bool, error)
		oldConnectFunc                   func(context.Context, *net.TCPListener, kubeclient.Client, *cobra.Command, string, string) error
	)

	BeforeEach(func() {
		oldProxyOnly = proxyOnly
		oldCustomPort = customPort
		oldListenAddress = listenAddress
		oldClientAndNamespaceFromContext = clientAndNamespaceFromContext
		oldConnectFunc = connectFunc
	})

	AfterEach(func() {
		proxyOnly = oldProxyOnly
		customPort = oldCustomPort
		listenAddress = oldListenAddress
		clientAndNamespaceFromContext = oldClientAndNamespaceFromContext
		connectFunc = oldConnectFunc
	})

	Describe("Run", func() {
		It("refreshes client before reconnect", func() {
			proxyOnly = true
			customPort = 0
			listenAddress = "127.0.0.1"

			var clientCalls int
			clientAndNamespaceFromContext = func(context.Context) (kubeclient.Client, string, bool, error) {
				clientCalls++
				return newFakeClient(), "default", false, nil
			}

			var connectCalls int
			connectFunc = func(_ context.Context, ln *net.TCPListener, _ kubeclient.Client, _ *cobra.Command, namespace, vmName string) error {
				connectCalls++
				Expect(ln).NotTo(BeNil())
				Expect(namespace).To(Equal("default"))
				Expect(vmName).To(Equal("test-vm"))
				if connectCalls == 1 {
					return errors.New("temporary error")
				}
				return nil
			}

			cmd := &cobra.Command{}
			stdout := &bytes.Buffer{}
			cmd.SetOut(stdout)
			cmd.SetErr(stdout)
			cmd.SetContext(context.Background())

			err := (&VNC{}).Run(cmd, []string{"test-vm"})
			Expect(err).NotTo(HaveOccurred())
			Expect(connectCalls).To(Equal(2))
			Expect(clientCalls).To(Equal(2))
		})
	})
})
