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

package kubeclient

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
)

func TestKubeclient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubeclient Config Suite")
}

var _ = Describe("ClientConfigFlagNames", func() {
	It("stays in sync with the flags DefaultClientConfig binds", func() {
		bound := pflag.NewFlagSet("bound", pflag.ContinueOnError)
		DefaultClientConfig(bound)

		var boundNames []string
		bound.VisitAll(func(f *pflag.Flag) {
			boundNames = append(boundNames, f.Name)
		})

		Expect(ClientConfigFlagNames()).To(ConsistOf(boundNames))
	})

	It("includes the core connection flags", func() {
		Expect(ClientConfigFlagNames()).To(ContainElements(
			"kubeconfig", "context", "server", "namespace",
		))
	})

	It("does not include klog flags, which DefaultClientConfig does not bind", func() {
		Expect(ClientConfigFlagNames()).NotTo(ContainElement("v"))
		Expect(ClientConfigFlagNames()).NotTo(ContainElement("vmodule"))
	})
})
