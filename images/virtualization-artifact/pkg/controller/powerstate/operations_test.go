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

package powerstate

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
)

const (
	testVMName      = "test-vm"
	testVMNamespace = "test-namespace"
	testNodeName    = "worker-01"
	testKVVMIUID    = types.UID("test-kvvmi-uid")
	testPodUID      = types.UID("test-pod-uid")
)

var testKey = types.NamespacedName{Name: testVMName, Namespace: testVMNamespace}

var _ = Describe("RestartVM", func() {
	newClient := func() (client.WithWatch, error) {
		kvvm := &virtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: testVMName, Namespace: testVMNamespace},
			TypeMeta:   metav1.TypeMeta{Kind: "VirtualMachine", APIVersion: virtv1.SchemeGroupVersion.String()},
		}
		kvvmi := &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{Name: testVMName, Namespace: testVMNamespace, UID: testKVVMIUID},
			TypeMeta:   metav1.TypeMeta{Kind: "VirtualMachineInstance", APIVersion: virtv1.SchemeGroupVersion.String()},
			Status: virtv1.VirtualMachineInstanceStatus{
				NodeName:   testNodeName,
				ActivePods: map[types.UID]string{testPodUID: testVMName},
			},
		}
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testVMName,
				Namespace: testVMNamespace,
				UID:       testPodUID,
				Labels: map[string]string{
					virtv1.AppLabel:       "virt-launcher",
					virtv1.CreatedByLabel: string(testKVVMIUID),
				},
			},
			Spec:     corev1.PodSpec{NodeName: testNodeName},
			TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: corev1.SchemeGroupVersion.String()},
		}
		return testutil.NewFakeClientWithObjects(kvvm, kvvmi, pod)
	}

	DescribeTable("adds stop and start requests and deletes the pod only on force",
		func(force bool) {
			c, err := newClient()
			Expect(err).NotTo(HaveOccurred())

			kvvm := &virtv1.VirtualMachine{}
			Expect(c.Get(context.Background(), testKey, kvvm)).To(Succeed())
			kvvmi := &virtv1.VirtualMachineInstance{}
			Expect(c.Get(context.Background(), testKey, kvvmi)).To(Succeed())

			Expect(RestartVM(context.Background(), c, kvvm, kvvmi, force)).To(Succeed())

			Expect(c.Get(context.Background(), testKey, kvvm)).To(Succeed())
			Expect(kvvm.Status.StateChangeRequests).To(HaveLen(2))
			Expect(kvvm.Status.StateChangeRequests[0].Action).To(Equal(virtv1.StopRequest))
			Expect(kvvm.Status.StateChangeRequests[0].UID).To(HaveValue(Equal(testKVVMIUID)))
			Expect(kvvm.Status.StateChangeRequests[1].Action).To(Equal(virtv1.StartRequest))

			err = c.Get(context.Background(), testKey, &corev1.Pod{})
			if force {
				Expect(k8serrors.IsNotFound(err)).To(BeTrue())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("without force", false),
		Entry("with force", true),
	)
})

var _ = Describe("StopVM", func() {
	// The finalizer keeps the VMI readable after deletion so the test can inspect its patched grace period.
	newKVVMI := func() *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:       testVMName,
				Namespace:  testVMNamespace,
				Finalizers: []string{"virtualization.deckhouse.io/test"},
			},
			TypeMeta: metav1.TypeMeta{Kind: "VirtualMachineInstance", APIVersion: virtv1.SchemeGroupVersion.String()},
			Spec:     virtv1.VirtualMachineInstanceSpec{TerminationGracePeriodSeconds: ptr.To(int64(60))},
		}
	}

	deletedGrace := func(c client.Client) *int64 {
		got := &virtv1.VirtualMachineInstance{}
		Expect(c.Get(context.Background(), testKey, got)).To(Succeed())
		Expect(got.DeletionTimestamp).NotTo(BeNil())
		return got.Spec.TerminationGracePeriodSeconds
	}

	It("keeps the grace period on a graceful stop", func() {
		c, err := testutil.NewFakeClientWithObjects(newKVVMI())
		Expect(err).NotTo(HaveOccurred())
		kvvmi := &virtv1.VirtualMachineInstance{}
		Expect(c.Get(context.Background(), testKey, kvvmi)).To(Succeed())

		Expect(StopVM(context.Background(), c, kvvmi, ptr.To(false))).To(Succeed())
		Expect(deletedGrace(c)).To(HaveValue(Equal(int64(60))))
	})

	It("zeroes the grace period on a force stop", func() {
		c, err := testutil.NewFakeClientWithObjects(newKVVMI())
		Expect(err).NotTo(HaveOccurred())
		kvvmi := &virtv1.VirtualMachineInstance{}
		Expect(c.Get(context.Background(), testKey, kvvmi)).To(Succeed())

		Expect(StopVM(context.Background(), c, kvvmi, ptr.To(true))).To(Succeed())
		Expect(deletedGrace(c)).To(HaveValue(Equal(int64(0))))
	})

	// Regression: force must shorten the grace period even when a graceful stop already left the VMI terminating,
	// otherwise the force stop is a no-op and the VM waits out the original grace period.
	It("zeroes the grace period on a force stop of an already terminating VMI", func() {
		c, err := testutil.NewFakeClientWithObjects(newKVVMI())
		Expect(err).NotTo(HaveOccurred())
		Expect(c.Delete(context.Background(), newKVVMI())).To(Succeed())
		terminating := &virtv1.VirtualMachineInstance{}
		Expect(c.Get(context.Background(), testKey, terminating)).To(Succeed())
		Expect(terminating.DeletionTimestamp).NotTo(BeNil())

		Expect(StopVM(context.Background(), c, terminating, ptr.To(true))).To(Succeed())
		Expect(deletedGrace(c)).To(HaveValue(Equal(int64(0))))
	})
})
