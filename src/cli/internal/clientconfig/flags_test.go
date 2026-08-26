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

package clientconfig

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

func TestClientConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Client Config Suite")
}

var _ = Describe("ForwardedFlags", func() {
	// newTree mimics the production layout: global client-config flags live on the parent
	// `v` command's persistent flags and are inherited by the child command.
	newTree := func() (root, child *cobra.Command) {
		root = &cobra.Command{Use: "v"}
		root.PersistentFlags().String("context", "", "")
		root.PersistentFlags().String("kubeconfig", "", "")
		root.PersistentFlags().String("server", "", "")
		root.PersistentFlags().StringP("namespace", "n", "", "")
		root.PersistentFlags().Bool("insecure-skip-tls-verify", false, "")
		root.PersistentFlags().String("token", "", "")
		child = &cobra.Command{Use: "ssh"}
		root.AddCommand(child)
		return root, child
	}

	It("returns nothing when the user set no flags", func() {
		_, child := newTree()
		Expect(ForwardedFlags(child, ShellQuote)).To(BeEmpty())
	})

	It("returns the cluster-selection flags the user set", func() {
		root, child := newTree()
		Expect(root.PersistentFlags().Set("context", "prod")).To(Succeed())
		Expect(root.PersistentFlags().Set("kubeconfig", "/etc/kube/config")).To(Succeed())
		Expect(root.PersistentFlags().Set("server", "https://api.prod:6443")).To(Succeed())

		Expect(ForwardedFlags(child, ShellQuote)).To(ConsistOf(
			"--context='prod'",
			"--server='https://api.prod:6443'",
			"--kubeconfig='/etc/kube/config'",
		))
	})

	It("does not return --namespace: callers carry it in the target", func() {
		root, child := newTree()
		Expect(root.PersistentFlags().Set("namespace", "other")).To(Succeed())

		Expect(ForwardedFlags(child, ShellQuote)).To(BeEmpty())
	})

	It("does not return sensitive or non-cluster-selection flags", func() {
		root, child := newTree()
		Expect(root.PersistentFlags().Set("token", "s3cr3t")).To(Succeed())
		Expect(root.PersistentFlags().Set("insecure-skip-tls-verify", "true")).To(Succeed())

		Expect(ForwardedFlags(child, ShellQuote)).To(BeEmpty())
	})

	It("applies the quoting the caller asked for", func() {
		root, child := newTree()
		Expect(root.PersistentFlags().Set("kubeconfig", "/home/user/my configs/kubeconfig")).To(Succeed())

		Expect(ForwardedFlags(child, ShellEscape)).To(ConsistOf(`--kubeconfig=/home/user/my\ configs/kubeconfig`))
	})
})

var _ = Describe("ShellQuote", func() {
	It("wraps a plain value in single quotes", func() {
		Expect(ShellQuote("prod")).To(Equal("'prod'"))
	})

	It("escapes embedded single quotes so /bin/sh re-parses them safely", func() {
		Expect(ShellQuote("a'b")).To(Equal(`'a'\''b'`))
	})
})

var _ = Describe("ShellEscape", func() {
	It("leaves a value made of safe characters alone", func() {
		Expect(ShellEscape("https://api.prod:6443")).To(Equal("https://api.prod:6443"))
	})

	It("escapes whitespace", func() {
		Expect(ShellEscape("my configs")).To(Equal(`my\ configs`))
	})

	// The value is parsed by shlex and then by /bin/sh, so a single quote has to leave the
	// quoted run of the enclosing layer, reach /bin/sh escaped and reopen the run.
	It("takes a single quote out of the enclosing quoted run", func() {
		Expect(ShellEscape("a'b")).To(Equal(`a\'\''b`))
	})

	It("escapes the characters that would end the command", func() {
		Expect(ShellEscape("a;rm -rf /")).To(Equal(`a\;rm\ -rf\ /`))
	})

	It("leaves non-ASCII characters as they are", func() {
		Expect(ShellEscape("münchen-東京")).To(Equal("münchen-東京"))
	})
})
