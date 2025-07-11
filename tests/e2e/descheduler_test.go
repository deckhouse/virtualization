/*
Copyright 2025 Flant JSC

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

package e2e

import (
	"context"
	"maps"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	"github.com/deckhouse/virtualization/tests/e2e/object"
)

var _ = Describe("Virtual machine reschedule [Descheduler]", SIGMigration(), ginkgoutil.CommonE2ETestDecorators(), func() {
	f := framework.NewFramework("vm-descheduler")

	skipIfDeschedulerNotExists := func() {
		res := f.Kubectl().GetResource(kc.ResourceDescheduler, "virtualization", kc.GetOptions{})
		if !res.WasSuccess() {
			Skip("Descheduler virtualization is not found")
		}
	}

	BeforeAll(func() {
		skipIfDeschedulerNotExists()
	})

	f.BeforeEach()
	f.AfterEach()

	Context("Check RemovePodsViolatingNodeAffinity (requiredDuringSchedulingIgnoredDuringExecution)", func() {
		const (
			prefixName           = "vm-descheduler-test"
			nodeLabel            = "vm-descheduler-test"
			deschedulerNamespace = "d8-descheduler"
		)

		var (
			deschedulerNamespaceOpt    = client.InNamespace(deschedulerNamespace)
			deschedulerPodsSelectorOpt = client.MatchingLabelsSelector{
				Selector: labels.SelectorFromSet(map[string]string{
					"app": "descheduler",
				}),
			}
			c = f.GenericClient()

			vd *virtv2.VirtualDisk
		)

		BeforeEach(func() {
			By("Create VirtualDisk")
			vd = object.NewHTTPVDUbuntu(prefixName, f.Namespace().Name)
			Eventually(func() error {
				return client.IgnoreAlreadyExists(c.Create(context.Background(), vd))
			}).WithTimeout(Timeout).WithPolling(1 * time.Second).Should(Succeed())
		})

		AfterEach(func() {
			By("Remove label from nodes")
			Eventually(func() error {
				nodes := &corev1.NodeList{}
				if err := c.List(context.Background(), nodes); err != nil {
					return err
				}
				for _, node := range nodes.Items {
					if _, ok := node.GetLabels()[nodeLabel]; !ok {
						continue
					}
					delete(node.GetLabels(), nodeLabel)
					if err := c.Update(context.Background(), &node); err != nil {
						return err
					}
				}
				return nil
			}).WithTimeout(Timeout).WithPolling(1 * time.Second).Should(Succeed())

			By("Remove VirtualDisk")
			Eventually(func() error {
				return c.Delete(context.Background(), vd)
			}).WithTimeout(Timeout).WithPolling(1 * time.Second).Should(Succeed())
		})

		It("Should migrate vm violating node affinity", func() {
			By("Label all nodes with `vm-descheduler-test` label")
			Eventually(func() error {
				nodes := &corev1.NodeList{}
				if err := c.List(context.Background(), nodes); err != nil {
					return err
				}
				for _, node := range nodes.Items {
					if _, ok := node.GetLabels()[nodeLabel]; ok {
						continue
					}
					maps.Copy(node.GetLabels(), map[string]string{nodeLabel: ""})
					if err := c.Update(context.Background(), &node); err != nil {
						return err
					}
				}
				return nil
			}).WithTimeout(Timeout).WithPolling(1 * time.Second).Should(Succeed())

			By("Create virtual machine with nodeSelector")
			genVM := object.NewMinimalVM(prefixName, f.Namespace().Name,
				vm.WithNodeSelector(map[string]string{nodeLabel: ""}),
				vm.WithDisks(vd),
			)
			Eventually(func() error {
				return c.Create(context.Background(), genVM)
			}).WithTimeout(Timeout).WithPolling(1 * time.Second).Should(Succeed())

			By("Check that vm is running")
			vmKey := client.ObjectKeyFromObject(genVM)
			var vm *virtv2.VirtualMachine
			Eventually(func() bool {
				err := c.Get(context.Background(), vmKey, vm)
				if err != nil {
					return false
				}
				return vm.Status.Phase == virtv2.MachineRunning
			}).WithTimeout(LongWaitDuration).WithPolling(1 * time.Second).Should(BeTrue())
			Expect(vm).NotTo(BeNil())

			By("Remove label from node")
			nodeName := vm.Status.Node
			Eventually(func() error {
				node := &corev1.Node{}
				if err := c.Get(context.Background(), client.ObjectKey{Name: nodeName}, node); err != nil {
					return err
				}
				delete(node.GetLabels(), nodeLabel)
				return c.Update(context.Background(), node)
			}).WithTimeout(Timeout).WithPolling(1 * time.Second).Should(Succeed())

			By("Restart pod descheduler")
			Eventually(func() error {
				pods := &corev1.PodList{}
				if err := c.List(context.Background(), pods, deschedulerNamespaceOpt, deschedulerPodsSelectorOpt); err != nil {
					return err
				}
				for _, pod := range pods.Items {
					if err := c.Delete(context.Background(), &pod); err != nil {
						return err
					}
				}
				return nil
			}).WithTimeout(Timeout).WithPolling(1 * time.Second).Should(Succeed())

			By("Wait until vm is rescheduled")
			Eventually(func() bool {
				err := c.Get(context.Background(), vmKey, vm)
				if err != nil {
					return false
				}
				newNodeName := vm.Status.Node
				return vm.Status.Phase == virtv2.MachineRunning && newNodeName != "" && newNodeName != nodeName
			}).WithTimeout(LongWaitDuration).WithPolling(5 * time.Second).Should(BeTrue())
		})
	})
})
