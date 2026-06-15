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

package ssh

import (
	"bytes"
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
)

func TestSSH(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SSH Command Suite")
}

var _ = Describe("SSH", func() {
	Describe("flattenSSHArgs", func() {
		It("returns an empty slice for empty input", func() {
			Expect(flattenSSHArgs(nil)).To(BeEmpty())
			Expect(flattenSSHArgs([]string{})).To(BeEmpty())
		})

		It("splits a single flag value by whitespace", func() {
			Expect(flattenSSHArgs([]string{
				"-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR",
			})).To(Equal([]string{
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "LogLevel=ERROR",
			}))
		})

		It("preserves each repeated flag as a separate entry", func() {
			Expect(flattenSSHArgs([]string{
				"-o StrictHostKeyChecking=no",
				"-o UserKnownHostsFile=/dev/null",
			})).To(Equal([]string{
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
			}))
		})

		It("mixes single-value and repeated flags keeping order", func() {
			Expect(flattenSSHArgs([]string{
				"-o X -o Y",
				"-o Z",
				"-o A -o B",
			})).To(Equal([]string{
				"-o", "X", "-o", "Y",
				"-o", "Z",
				"-o", "A", "-o", "B",
			}))
		})

		It("collapses runs of spaces and tabs", func() {
			Expect(flattenSSHArgs([]string{"  -o   X  \t  -o  Y  "})).To(Equal([]string{
				"-o", "X", "-o", "Y",
			}))
		})
	})

	Describe("WarnDeprecatedSSHFlags", func() {
		var captured *bytes.Buffer

		BeforeEach(func() {
			captured = &bytes.Buffer{}
		})

		runWith := func(setup func(*cobra.Command)) string {
			cmd := NewCommand()
			cmd.SetErr(captured)
			setup(cmd)
			WarnDeprecatedSSHFlags(cmd)
			return captured.String()
		}

		It("emits a warning when --local-ssh is set", func() {
			out := runWith(func(cmd *cobra.Command) {
				Expect(cmd.Flags().Set("local-ssh", "true")).To(Succeed())
			})
			Expect(out).To(ContainSubstring("--local-ssh is deprecated"))
		})

		It("emits a warning when --local-ssh-opts is set and points users to --ssh-args", func() {
			out := runWith(func(cmd *cobra.Command) {
				Expect(cmd.Flags().Set("local-ssh-opts", "-o X")).To(Succeed())
			})
			Expect(out).To(ContainSubstring("--local-ssh-opts is deprecated"))
			Expect(out).To(ContainSubstring("--ssh-args"))
		})

		It("stays silent for --command and -c, since they remain supported", func() {
			out := runWith(func(cmd *cobra.Command) {
				Expect(cmd.Flags().Set("command", "ls")).To(Succeed())
			})
			Expect(out).To(BeEmpty())
		})

		It("stays silent for the new --ssh-args flag", func() {
			out := runWith(func(cmd *cobra.Command) {
				Expect(cmd.Flags().Set("ssh-args", "-o StrictHostKeyChecking=no")).To(Succeed())
			})
			Expect(out).To(BeEmpty())
		})

		It("stays silent when no flags are changed", func() {
			out := runWith(func(_ *cobra.Command) {})
			Expect(out).To(BeEmpty())
		})
	})

	Describe("NewCommand", func() {
		It("registers the new --ssh-args flag and drops the old --ssh-opts", func() {
			cmd := NewCommand()
			Expect(cmd.Flags().Lookup("ssh-args")).NotTo(BeNil())
			Expect(cmd.Flags().Lookup("ssh-opts")).To(BeNil())
		})

		It("exposes --command and its -c short alias as supported flags", func() {
			cmd := NewCommand()
			Expect(cmd.Flags().Lookup("command")).NotTo(BeNil())
			Expect(cmd.Flags().ShorthandLookup("c")).NotTo(BeNil())
		})
	})

	Describe("ResolveDefaultNamespace", func() {
		newCtx := func(ns string) context.Context {
			yaml := []byte(`
apiVersion: v1
kind: Config
current-context: test
clusters:
- name: test-cluster
  cluster:
    server: https://localhost:443
contexts:
- name: test
  context:
    cluster: test-cluster
    user: test-user
    namespace: ` + ns + `
users:
- name: test-user
`)
			clientConfig, err := clientcmd.NewClientConfigFromBytes(yaml)
			Expect(err).NotTo(HaveOccurred())
			return clientconfig.NewContext(context.Background(), clientConfig)
		}

		It("uses the namespace from the kubeconfig context", func() {
			ctx := newCtx("vm-team")
			ns, err := clientconfig.NamespaceFromContext(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal("vm-team"))
		})

		It("returns \"default\" when no namespace is configured in the kubeconfig", func() {
			yaml := []byte(`
apiVersion: v1
kind: Config
current-context: test
clusters:
- name: test-cluster
  cluster:
    server: https://localhost:443
contexts:
- name: test
  context:
    cluster: test-cluster
    user: test-user
users:
- name: test-user
`)
			clientConfig, err := clientcmd.NewClientConfigFromBytes(yaml)
			Expect(err).NotTo(HaveOccurred())
			ctx := clientconfig.NewContext(context.Background(), clientConfig)

			ns, err := clientconfig.NamespaceFromContext(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal("default"))
		})

		It("returns an error when no client config is in the context", func() {
			ns, err := clientconfig.NamespaceFromContext(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(ns).To(BeEmpty())
		})
	})
})
