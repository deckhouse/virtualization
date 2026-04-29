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

package console

import (
	"context"
	"errors"
	"net"
	"os"
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

func TestConsole(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Console Command Suite")
}

var _ = Describe("Console", func() {
	var (
		oldStdin                         *os.File
		oldClientAndNamespaceFromContext func(context.Context) (kubeclient.Client, string, bool, error)
		oldConnectFunc                   func(context.Context, string, string, kubeclient.Client, time.Duration, <-chan []byte, <-chan struct{}) error
	)

	BeforeEach(func() {
		oldStdin = os.Stdin
		oldClientAndNamespaceFromContext = clientAndNamespaceFromContext
		oldConnectFunc = connectFunc
	})

	AfterEach(func() {
		os.Stdin = oldStdin
		clientAndNamespaceFromContext = oldClientAndNamespaceFromContext
		connectFunc = oldConnectFunc
	})

	Describe("Run", func() {
		It("refreshes client before reconnect", func() {
			stdinReader, stdinWriter, err := os.Pipe()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				_ = stdinReader.Close()
			})
			DeferCleanup(func() {
				_ = stdinWriter.Close()
			})
			os.Stdin = stdinReader

			var clientCalls int
			clientAndNamespaceFromContext = func(context.Context) (kubeclient.Client, string, bool, error) {
				clientCalls++
				return newFakeClient(), "default", false, nil
			}

			var connectCalls int
			connectFunc = func(_ context.Context, name, namespace string, _ kubeclient.Client, _ time.Duration, _ <-chan []byte, _ <-chan struct{}) error {
				connectCalls++
				Expect(namespace).To(Equal("default"))
				Expect(name).To(Equal("test-vm"))
				if connectCalls == 1 {
					return errors.New("temporary error")
				}
				return nil
			}

			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())

			go func() {
				_ = stdinWriter.Close()
			}()

			err = (&Console{timeout: time.Second}).Run(cmd, []string{"test-vm"})
			Expect(err).NotTo(HaveOccurred())
			Expect(connectCalls).To(Equal(2))
			Expect(clientCalls).To(Equal(2))
		})
	})

	Describe("ShouldWaitErr", func() {
		It("returns true for abnormal closure errors", func() {
			Expect(ShouldWaitErr(&net.OpError{Err: errors.New("Internal error")})).To(BeTrue())
		})
	})
})
