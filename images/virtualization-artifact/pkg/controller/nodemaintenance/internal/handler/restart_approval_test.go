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

package handler

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("RestartApprovalHandler", func() {
	const (
		nodeName  = "node1"
		namespace = "default"
	)

	ctx := testutil.ContextBackgroundWithNoOpLogger()

	newNode := func(annotationsMap map[string]string) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: nodeName, Annotations: annotationsMap},
			Spec:       corev1.NodeSpec{Unschedulable: true},
		}
	}

	newVM := func(reason vmcondition.EvictionRequiredReason) *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty("vm", namespace)
		vm.Status.Node = nodeName
		if reason != "" {
			vm.Status.Conditions = append(vm.Status.Conditions, metav1.Condition{
				Type:   vmcondition.TypeEvictionRequired.String(),
				Status: metav1.ConditionTrue,
				Reason: reason.String(),
			})
		}
		return vm
	}

	handleNode := func(node *corev1.Node, objs ...client.Object) *corev1.Node {
		fakeClient, err := testutil.NewFakeClientWithObjects(append([]client.Object{node}, objs...)...)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewRestartApprovalHandler(fakeClient).Handle(ctx, node)
		Expect(err).NotTo(HaveOccurred())

		updated := &corev1.Node{}
		Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(node), updated)).To(Succeed())
		return updated
	}

	// The module asks first: an administrator sees on the node itself that a decision is expected
	// from him, not only in an alert.
	It("asks for the permission while a machine holds the node", func() {
		node := handleNode(newNode(nil), newVM(vmcondition.ReasonEvictionBlocked))
		Expect(node.GetAnnotations()).To(HaveKey(annotations.AnnNodeVMRestartRequired))
	})

	It("asks nothing while every machine can leave the node alive", func() {
		node := handleNode(newNode(nil), newVM(vmcondition.ReasonEvictionRequired))
		Expect(node.GetAnnotations()).NotTo(HaveKey(annotations.AnnNodeVMRestartRequired))
	})

	// An approval given while planning the works waits for the maintenance instead of being taken
	// for a leftover of a finished one.
	It("keeps an approval given ahead of the works", func() {
		node := handleNode(newNode(map[string]string{annotations.AnnNodeVMRestartApproved: ""}))
		Expect(node.GetAnnotations()).To(HaveKey(annotations.AnnNodeVMRestartApproved))
		Expect(node.GetAnnotations()).NotTo(HaveKey(annotations.AnnNodeVMRestartRequired))
	})

	It("keeps the dialogue while the machine is still being restarted", func() {
		node := handleNode(newNode(map[string]string{
			annotations.AnnNodeVMRestartRequired: "",
			annotations.AnnNodeVMRestartApproved: "",
		}), newVM(vmcondition.ReasonRestartRequired))
		Expect(node.GetAnnotations()).To(HaveKey(annotations.AnnNodeVMRestartRequired))
		Expect(node.GetAnnotations()).To(HaveKey(annotations.AnnNodeVMRestartApproved))
	})

	// The node is released, so the approval is spent: the next maintenance asks again. The machines
	// may well be running elsewhere by then, which is why the node is watched on its own.
	It("spends the approval once the node is released", func() {
		node := handleNode(newNode(map[string]string{
			annotations.AnnNodeVMRestartRequired: "",
			annotations.AnnNodeVMRestartApproved: "",
		}))
		Expect(node.GetAnnotations()).NotTo(HaveKey(annotations.AnnNodeVMRestartRequired))
		Expect(node.GetAnnotations()).NotTo(HaveKey(annotations.AnnNodeVMRestartApproved))
	})

	It("withdraws the request even when nobody answered it", func() {
		node := handleNode(newNode(map[string]string{annotations.AnnNodeVMRestartRequired: ""}))
		Expect(node.GetAnnotations()).NotTo(HaveKey(annotations.AnnNodeVMRestartRequired))
	})
})
