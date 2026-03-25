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

package vm

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("HotplugPod", func() {
	var (
		f  = framework.NewFramework("hotplug-pod")
		vi *v1alpha2.VirtualImage
	)

	BeforeEach(func() {
		f.Before()
		DeferCleanup(f.After)

		newVI := object.NewGeneratedHTTPVIUbuntu("hotplug-pod-", f.Namespace().Name)
		newVI, err := f.VirtClient().VirtualImages(f.Namespace().Name).Create(context.Background(), newVI, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		f.DeferDelete(newVI)
		vi = newVI
	})

	It("Should protect hotplug pod", func() {
		var (
			vm    *v1alpha2.VirtualMachine
			blank *v1alpha2.VirtualDisk
		)
		By("Create VM", func() {
			root := object.NewVDFromVI("root", f.Namespace().Name, vi)
			blank = object.NewBlankVD("blank", f.Namespace().Name, nil, ptr.To(resource.MustParse("100Mi")))
			Expect(f.CreateWithDeferredDeletion(context.Background(), root, blank)).To(Succeed())

			var err error
			vm = object.NewMinimalVM("hotplug-pod-", f.Namespace().Name, vmbuilder.WithDisks(root))
			vm, err = f.VirtClient().VirtualMachines(f.Namespace().Name).Create(context.Background(), vm, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vm)
		})

		By("Wait until VM agent is ready", func() {
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(vm), framework.LongTimeout)
		})

		By("Attaching disk", func() {
			vmbda := object.NewVMBDAFromDisk(vm.Name, vm.Name, blank)
			Expect(f.CreateWithDeferredDeletion(context.Background(), vmbda)).To(Succeed())
			util.UntilObjectPhase(string(v1alpha2.BlockDeviceAttachmentPhaseAttached), framework.MiddleTimeout, vmbda)
		})

		By("Evict hp pod", func() {
			pods, err := f.KubeClient().CoreV1().Pods(f.Namespace().Name).List(context.Background(), metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(map[string]string{
					"kubevirt.internal.virtualization.deckhouse.io": "d8v-hotplug-disk",
				}).String(),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(pods.Items).To(HaveLen(1))

			pod := pods.Items[0]

			err = f.KubeClient().CoreV1().Pods(pod.GetNamespace()).EvictV1(context.Background(), &policyv1.Eviction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pod.GetName(),
					Namespace: pod.GetNamespace(),
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot evict hotplug pod"))
		})
	})
})
