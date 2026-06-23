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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

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
	virtualizationv1alpha2.VirtualizationV1alpha2Interface
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		Clientset:                       k8sfake.NewSimpleClientset(),
		VirtualizationV1alpha2Interface: virtualizationfake.NewSimpleClientset().VirtualizationV1alpha2(),
	}
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

		It("keeps proxy listener alive until context is canceled", func() {
			proxyOnly = true
			customPort = 0
			listenAddress = "127.0.0.1"

			clientAndNamespaceFromContext = func(context.Context) (kubeclient.Client, string, bool, error) {
				return newFakeClient(), "default", false, nil
			}

			connectDone := make(chan struct{})
			connectFunc = func(ctx context.Context, ln *net.TCPListener, _ kubeclient.Client, cmd *cobra.Command, _, _ string) error {
				// Mirror real connect(): in proxy-only mode the listener must
				// stay alive until ctx is canceled, not return immediately.
				port := ln.Addr().(*net.TCPAddr).Port
				Expect(port).To(BeNumerically(">", 0))
				fmt.Fprintf(cmd.OutOrStdout(), "{\"port\":%d}\n", port)
				<-ctx.Done()
				close(connectDone)
				return ctx.Err()
			}

			ctx, cancel := context.WithCancel(context.Background())
			cmd := &cobra.Command{}
			stdout := &bytes.Buffer{}
			cmd.SetOut(stdout)
			cmd.SetErr(stdout)
			cmd.SetContext(ctx)

			runDone := make(chan error, 1)
			go func() { runDone <- (&VNC{}).Run(cmd, []string{"test-vm"}) }()

			// Listener must be reachable after the port is printed.
			Eventually(stdout).Should(ContainSubstring(`"port"`))
			var port int
			for _, line := range bytes.Split(stdout.Bytes(), []byte("\n")) {
				if bytes.Contains(line, []byte(`"port"`)) {
					Expect(json.Unmarshal(line, &struct {
						Port *int `json:"port"`
					}{Port: &port})).To(Succeed())
					break
				}
			}

			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
			Expect(err).NotTo(HaveOccurred())
			conn.Close()

			// Run must not return on its own: proxy stays up until ctx cancel.
			Consistently(runDone, 200*time.Millisecond).ShouldNot(Receive())

			cancel()
			Eventually(connectDone).Should(BeClosed())
			Eventually(runDone).Should(Receive())
		})
	})
})
