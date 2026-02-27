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

package internal

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("SyncMetadataHandler", func() {
	const (
		name      = "vm-metadata-sync"
		namespace = "default"

		testAnnoName  = "testAnnoName"
		testAnnoValue = "testAnnoValue"

		testLabelName  = "testLabelName"
		testLabelValue = "testLabelValue"

		kubecltLastAppliedConfLabel = "kubectl.kubernetes.io/last-applied-configuration"

		testNetworkAnnoValue   = `[{"type":"ClusterNetwork","name":"test","ifName":"veth_cn81b2c569"}]`
		liveMigrationAnnoValue = "true"
		ipAddressAnnoValue     = "10.66.10.1"

		skipSecurityCheckLabelValue = "true"
	)

	var (
		ctx        context.Context
		fakeClient client.WithWatch
		vmState    state.VirtualMachineState
		recorder   *eventrecord.EventRecorderLoggerMock
	)

	BeforeEach(func() {
		ctx = testutil.ContextBackgroundWithNoOpLogger()
		fakeClient = nil
		vmState = nil
		recorder = &eventrecord.EventRecorderLoggerMock{
			EventFunc:       func(_ client.Object, _, _, _ string) {},
			EventfFunc:      func(_ client.Object, _, _, _ string, _ ...interface{}) {},
			WithLoggingFunc: func(logger eventrecord.InfoLogger) eventrecord.EventRecorderLogger { return recorder },
		}
	})

	AfterEach(func() {
		fakeClient = nil
		vmState = nil
		recorder = nil
	})

	newVM := func() *v1alpha2.VirtualMachine {
		vm := vmbuilder.NewEmpty(name, namespace)
		vm.Labels = map[string]string{
			testLabelName: testLabelValue,
		}
		vm.Annotations = map[string]string{
			testAnnoName: testAnnoValue,
		}
		vm.Status.Resources = v1alpha2.ResourcesStatus{
			CPU: v1alpha2.CPUStatus{
				RuntimeOverhead: resource.MustParse("100m"),
			},
			Memory: v1alpha2.MemoryStatus{
				RuntimeOverhead: resource.MustParse("128Mi"),
			},
		}

		return vm
	}

	newKVVM := func(vm *v1alpha2.VirtualMachine) *virtv1.VirtualMachine {
		kvvm := newEmptyKVVM(vm.Name, vm.Namespace)
		kvvm.Spec = virtv1.VirtualMachineSpec{
			Template: &virtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
			},
		}
		kvvm.Spec.Template.ObjectMeta.Annotations = make(map[string]string, len(vm.Annotations))
		kvvm.Spec.Template.ObjectMeta.Labels = make(map[string]string, len(vm.Labels))

		kvvm.Spec.Template.ObjectMeta.Annotations[annotations.AnnNetworksSpec] = testNetworkAnnoValue
		kvvm.Spec.Template.ObjectMeta.Annotations[virtv1.AllowPodBridgeNetworkLiveMigrationAnnotation] = liveMigrationAnnoValue
		kvvm.Spec.Template.ObjectMeta.Annotations[netmanager.AnnoIPAddressCNIRequest] = ipAddressAnnoValue

		kvvm.Spec.Template.ObjectMeta.Labels[annotations.SkipPodSecurityStandardsCheckLabel] = skipSecurityCheckLabelValue

		return kvvm
	}

	validateObjMetadata := func(obj client.Object) {
		Expect(obj.GetAnnotations()).To(And(
			HaveKeyWithValue(testAnnoName, testAnnoValue),
			Not(HaveKey(kubecltLastAppliedConfLabel)),
			Not(HaveKey(annotations.LastPropagatedVMAnnotationsAnnotation)),
			Not(HaveKey(annotations.LastPropagatedVMLabelsAnnotation)),
		))
		Expect(obj.GetLabels()).To(And(
			HaveKeyWithValue(testLabelName, testLabelValue),
			HaveKeyWithValue(annotations.InhibitNodeShutdownLabel, ""),
		))
	}

	validateObjResourceStatusLabels := func(obj client.Object, vm *v1alpha2.VirtualMachine) {
		if !vm.Status.Resources.CPU.RuntimeOverhead.IsZero() {
			Expect(obj.GetLabels()).To(And(
				HaveKeyWithValue(annotations.QuotaDiscountCPU, vm.Status.Resources.CPU.RuntimeOverhead.String()),
			))
		}
		if !vm.Status.Resources.Memory.RuntimeOverhead.IsZero() {
			Expect(obj.GetLabels()).To(HaveKeyWithValue(annotations.QuotaDiscountMemory, vm.Status.Resources.Memory.RuntimeOverhead.String()))
		}
	}

	validateKVVMMetadata := func(kvvm *virtv1.VirtualMachine, vm *v1alpha2.VirtualMachine) {
		// Validate KVVM Metadata
		Expect(kvvm.GetAnnotations()).To(And(
			HaveKeyWithValue(testAnnoName, testAnnoValue),
			Not(HaveKey(kubecltLastAppliedConfLabel)),
			HaveKey(annotations.LastPropagatedVMAnnotationsAnnotation),
			HaveKey(annotations.LastPropagatedVMLabelsAnnotation),
		))
		Expect(kvvm.GetLabels()).To(And(
			HaveKeyWithValue(testLabelName, testLabelValue),
			HaveKeyWithValue(annotations.InhibitNodeShutdownLabel, ""),
		))

		// Validate KVVM Spec Template Metadata
		Expect(kvvm.Spec.Template.ObjectMeta.Annotations).To(And(
			HaveKeyWithValue(testAnnoName, testAnnoValue),
			HaveKeyWithValue(annotations.AnnNetworksSpec, testNetworkAnnoValue),
			HaveKeyWithValue(virtv1.AllowPodBridgeNetworkLiveMigrationAnnotation, liveMigrationAnnoValue),
			HaveKeyWithValue(netmanager.AnnoIPAddressCNIRequest, ipAddressAnnoValue),
			Not(HaveKey(kubecltLastAppliedConfLabel)),
			Not(HaveKey(annotations.LastPropagatedVMAnnotationsAnnotation)),
			Not(HaveKey(annotations.LastPropagatedVMLabelsAnnotation)),
		))
		Expect(kvvm.Spec.Template.ObjectMeta.Labels).To(And(
			HaveKeyWithValue(testLabelName, testLabelValue),
			HaveKeyWithValue(annotations.SkipPodSecurityStandardsCheckLabel, skipSecurityCheckLabelValue),
			HaveKeyWithValue(annotations.InhibitNodeShutdownLabel, ""),
		))

		// Validate Resource Status Annotations
		validateObjResourceStatusLabels(kvvm, vm)
	}

	Describe("Propagating VM metadata to KVVM, KVVMI and Pod", func() {
		It("handles a virtual machine metadata updating", func() {
			vm := newVM()
			kvvm := newKVVM(vm)
			kvvmi := newEmptyKVVMI(name, namespace)
			pod := newEmptyPOD(name, namespace, vm.Name)
			pod.Status.Phase = corev1.PodRunning

			vm.Status.VirtualMachinePods = []v1alpha2.VirtualMachinePod{
				{
					Name:   pod.Name,
					Active: true,
				},
			}

			fakeClient, _, vmState = setupEnvironment(vm, kvvm, kvvmi, pod)
			h := NewSyncMetadataHandler(fakeClient)
			_, err := h.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			err = fakeClient.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: vm.Name}, vm)
			Expect(err).NotTo(HaveOccurred())

			By("Validate KVVM metadata", func() {
				err = fakeClient.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: vm.Name}, kvvm)
				Expect(err).NotTo(HaveOccurred())
				validateKVVMMetadata(kvvm, vm)
			})

			By("Validate KVVMI Metadata", func() {
				err = fakeClient.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: vm.Name}, kvvmi)
				Expect(err).NotTo(HaveOccurred())
				validateObjMetadata(kvvmi)
				validateObjResourceStatusLabels(kvvmi, vm)
			})

			By("Validate Pod Metadata", func() {
				err = fakeClient.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: vm.Name}, pod)
				Expect(err).NotTo(HaveOccurred())
				validateObjMetadata(pod)
				validateObjResourceStatusLabels(pod, vm)
			})
		})
	})
})
