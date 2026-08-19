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
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmbdaobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmbda"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

var _ = Describe("HotplugPod", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		vi  *v1alpha2.VirtualImage
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("hotplug-pod")
		f.Before()
		DeferCleanup(f.After)

		newVI := object.NewGeneratedHTTPVICustomBIOS("hotplug-pod-", f.Namespace().Name)
		newVI, err := f.VirtClient().VirtualImages(f.Namespace().Name).Create(ctx, newVI, metav1.CreateOptions{})
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
			root := object.NewVDFromVI("root", f.Namespace().Name, vi, vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))))
			blank = object.NewBlankVD("blank", f.Namespace().Name, nil, ptr.To(resource.MustParse(vdCustomImageSize)))
			Expect(f.CreateWithDeferredDeletion(ctx, root, blank)).To(Succeed())

			var err error
			// The custom image has no cloud-init; the guest agent is baked
			// in, so no provisioning is needed.
			vm = object.NewMinimalVM("hotplug-pod-", f.Namespace().Name, vmbuilder.WithDisks(root))
			vm, err = f.VirtClient().VirtualMachines(f.Namespace().Name).Create(ctx, vm, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vm)
		})

		By("Wait until VM agent is ready", func() {
			vmObs := vmobs.StartObserver(ctx, f, vm)
			vmObs.Never(vmobs.BeFailed())
			err := vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Attaching disk", func() {
			vmbda := object.NewVMBDAFromDisk(vm.Name, vm.Name, blank)
			vmbdaObs := vmbdaobs.StartObserver(ctx, f, vmbda)
			Expect(f.CreateWithDeferredDeletion(ctx, vmbda)).To(Succeed())
			// The first attachment waits out the blank disk provisioning and the CSI
			// attach of the hotplug pod, which take minutes under a parallel run.
			err := vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Evict hp pod", func() {
			pods, err := f.KubeClient().CoreV1().Pods(f.Namespace().Name).List(ctx, metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(map[string]string{
					"kubevirt.internal.virtualization.deckhouse.io": "d8v-hotplug-disk",
				}).String(),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(pods.Items).To(HaveLen(1))

			pod := pods.Items[0]

			err = f.KubeClient().CoreV1().Pods(pod.GetNamespace()).EvictV1(ctx, &policyv1.Eviction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pod.GetName(),
					Namespace: pod.GetNamespace(),
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot evict hotplug pod"))
		})
	})

	It("Should propagate priority class to hotplug pod", func() {
		var (
			vm            *v1alpha2.VirtualMachine
			blank         *v1alpha2.VirtualDisk
			priorityClass *schedulingv1.PriorityClass
		)
		By("Create priority class", func() {
			// A dedicated class instead of a cluster one (develop etc.): the globalDefault
			// admission would put the default class on the pod even without inheritance,
			// making the test pass vacuously.
			priorityClass = &schedulingv1.PriorityClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "e2e-hotplug-" + f.Namespace().Name,
				},
				Value:            100,
				PreemptionPolicy: ptr.To(corev1.PreemptNever),
			}
			Expect(f.CreateWithDeferredDeletion(ctx, priorityClass)).To(Succeed())
		})

		By("Create VM", func() {
			root := object.NewVDFromVI("root", f.Namespace().Name, vi, vdbuilder.WithSize(ptr.To(resource.MustParse("400Mi"))))
			blank = object.NewBlankVD("blank", f.Namespace().Name, nil, ptr.To(resource.MustParse("100Mi")))
			Expect(f.CreateWithDeferredDeletion(ctx, root, blank)).To(Succeed())

			var err error
			vm = object.NewMinimalVM("hotplug-pod-priority-", f.Namespace().Name,
				vmbuilder.WithDisks(root),
				vmbuilder.WithPriorityClassName(priorityClass.Name),
			)
			vm, err = f.VirtClient().VirtualMachines(f.Namespace().Name).Create(ctx, vm, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			f.DeferDelete(vm)
		})

		By("Wait until VM agent is ready", func() {
			vmObs := vmobs.StartObserver(ctx, f, vm)
			vmObs.Never(vmobs.BeFailed())
			err := vmObs.WaitFor(vmobs.BeAgentReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Attaching disk", func() {
			vmbda := object.NewVMBDAFromDisk(vm.Name, vm.Name, blank)
			vmbdaObs := vmbdaobs.StartObserver(ctx, f, vmbda)
			Expect(f.CreateWithDeferredDeletion(ctx, vmbda)).To(Succeed())
			// The first attachment waits out the blank disk provisioning and the CSI
			// attach of the hotplug pod, which take minutes under a parallel run.
			err := vmbdaObs.WaitFor(vmbdaobs.BeAttached(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Check hotplug pod priority class", func() {
			pods, err := f.KubeClient().CoreV1().Pods(f.Namespace().Name).List(ctx, metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(map[string]string{
					"kubevirt.internal.virtualization.deckhouse.io": "d8v-hotplug-disk",
				}).String(),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(pods.Items).To(HaveLen(1))

			Expect(pods.Items[0].Spec.PriorityClassName).To(Equal(priorityClass.Name))
		})
	})
})
