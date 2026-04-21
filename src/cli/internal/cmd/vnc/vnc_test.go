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

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes/fake"

	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
)

type fakeKubeclient struct {
	*fake.Clientset
}

func (f *fakeKubeclient) ClusterVirtualImages() virtualizationv1alpha2.ClusterVirtualImageInterface {
	return nil
}

func (f *fakeKubeclient) VirtualMachines(namespace string) virtualizationv1alpha2.VirtualMachineInterface {
	return nil
}

func (f *fakeKubeclient) VirtualImages(namespace string) virtualizationv1alpha2.VirtualImageInterface {
	return nil
}

func (f *fakeKubeclient) VirtualDisks(namespace string) virtualizationv1alpha2.VirtualDiskInterface {
	return nil
}

func (f *fakeKubeclient) VirtualMachineBlockDeviceAttachments(namespace string) virtualizationv1alpha2.VirtualMachineBlockDeviceAttachmentInterface {
	return nil
}

func (f *fakeKubeclient) VirtualMachineIPAddresses(namespace string) virtualizationv1alpha2.VirtualMachineIPAddressInterface {
	return nil
}

func (f *fakeKubeclient) VirtualMachineIPAddressLeases() virtualizationv1alpha2.VirtualMachineIPAddressLeaseInterface {
	return nil
}

func (f *fakeKubeclient) VirtualMachineOperations(namespace string) virtualizationv1alpha2.VirtualMachineOperationInterface {
	return nil
}

func (f *fakeKubeclient) VirtualMachineClasses() virtualizationv1alpha2.VirtualMachineClassInterface {
	return nil
}

func (f *fakeKubeclient) VirtualMachineMACAddresses(namespace string) virtualizationv1alpha2.VirtualMachineMACAddressInterface {
	return nil
}

func (f *fakeKubeclient) VirtualMachineMACAddressLeases() virtualizationv1alpha2.VirtualMachineMACAddressLeaseInterface {
	return nil
}

func (f *fakeKubeclient) NodeUSBDevices() virtualizationv1alpha2.NodeUSBDeviceInterface {
	return nil
}

func (f *fakeKubeclient) USBDevices(namespace string) virtualizationv1alpha2.USBDeviceInterface {
	return nil
}

func TestRunRefreshesClientBeforeReconnect(t *testing.T) {
	oldProxyOnly := proxyOnly
	oldCustomPort := customPort
	oldListenAddress := listenAddress
	oldClientAndNamespaceFromContext := clientAndNamespaceFromContext
	oldConnectFunc := connectFunc
	defer func() {
		proxyOnly = oldProxyOnly
		customPort = oldCustomPort
		listenAddress = oldListenAddress
		clientAndNamespaceFromContext = oldClientAndNamespaceFromContext
		connectFunc = oldConnectFunc
	}()

	proxyOnly = true
	customPort = 0
	listenAddress = "127.0.0.1"

	var clientCalls int
	clientAndNamespaceFromContext = func(context.Context) (kubeclient.Client, string, bool, error) {
		clientCalls++
		return &fakeKubeclient{Clientset: fake.NewSimpleClientset()}, "default", false, nil
	}

	var connectCalls int
	connectFunc = func(_ context.Context, ln *net.TCPListener, _ kubeclient.Client, _ *cobra.Command, namespace, vmName string) error {
		connectCalls++
		if ln == nil {
			t.Fatal("listener must not be nil")
		}
		if namespace != "default" {
			t.Fatalf("unexpected namespace: %s", namespace)
		}
		if vmName != "test-vm" {
			t.Fatalf("unexpected vm name: %s", vmName)
		}
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
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if connectCalls != 2 {
		t.Fatalf("expected 2 connect attempts, got %d", connectCalls)
	}
	if clientCalls != 2 {
		t.Fatalf("expected client to be refreshed before each reconnect, got %d calls", clientCalls)
	}
}
