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

package ansibleinventory

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

func TestAnsibleInventory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ansible Inventory Command Suite")
}

var _ = Describe("sshCommonArgs", func() {
	// newTree mimics the production layout: global client-config flags live on the parent
	// `v` command's persistent flags and are inherited by the `ansible-inventory` child.
	newTree := func() (root, inventoryCmd *cobra.Command) {
		root = &cobra.Command{Use: "v"}
		root.PersistentFlags().String("context", "", "")
		root.PersistentFlags().String("kubeconfig", "", "")
		root.PersistentFlags().String("server", "", "")
		root.PersistentFlags().String("token", "", "")
		inventoryCmd = &cobra.Command{Use: "ansible-inventory"}
		root.AddCommand(inventoryCmd)
		return root, inventoryCmd
	}

	It("proxies through port-forward with the ansible host and port placeholders", func() {
		_, inventoryCmd := newTree()
		Expect(sshCommonArgs(inventoryCmd)).To(Equal(`-o ProxyCommand='d8 v port-forward --stdio=true %h %p'`))
	})

	It("carries the cluster-selection flags into the proxy command", func() {
		root, inventoryCmd := newTree()
		Expect(root.PersistentFlags().Set("context", "prod")).To(Succeed())
		Expect(root.PersistentFlags().Set("server", "https://api.prod:6443")).To(Succeed())

		args := sshCommonArgs(inventoryCmd)
		Expect(args).To(ContainSubstring("--context=prod"))
		Expect(args).To(ContainSubstring("--server=https://api.prod:6443"))
	})

	It("keeps the single quotes of the ProxyCommand value intact", func() {
		root, inventoryCmd := newTree()
		Expect(root.PersistentFlags().Set("kubeconfig", "/home/user/my configs/kubeconfig")).To(Succeed())

		args := sshCommonArgs(inventoryCmd)
		// Backslash escaping, not quoting: a nested single quote would end the value early.
		Expect(args).To(Equal(`-o ProxyCommand='d8 v port-forward --stdio=true %h %p --kubeconfig=/home/user/my\ configs/kubeconfig'`))
	})

	It("does not leak sensitive flags into the inventory", func() {
		root, inventoryCmd := newTree()
		Expect(root.PersistentFlags().Set("token", "s3cr3t")).To(Succeed())

		Expect(sshCommonArgs(inventoryCmd)).NotTo(ContainSubstring("s3cr3t"))
	})
})
