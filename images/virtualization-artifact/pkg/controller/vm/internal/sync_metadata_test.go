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
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/netmanager"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
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

		// A hung source pod keeps the Running phase, so it goes on being patched with the
		// propagated labels: the inhibit label has to be stripped from it explicitly.
		It("strips the inhibit-node-shutdown label from a pod left behind by a migration", func() {
			const (
				sourceNode = "node-a"
				targetNode = "node-b"
			)

			vm := newVM()
			kvvm := newKVVM(vm)

			kvvmi := newEmptyKVVMI(name, namespace)
			kvvmi.Status.NodeName = targetNode
			kvvmi.Status.ActivePods = map[types.UID]string{
				"source-uid": sourceNode,
				"target-uid": targetNode,
			}
			kvvmi.Status.MigrationState = &virtv1.VirtualMachineInstanceMigrationState{
				SourcePod: name + "-source",
				TargetPod: name + "-target",
				Completed: true,
			}

			newRunningPod := func(podName, node string, uid types.UID) *corev1.Pod {
				pod := newEmptyPOD(podName, namespace, vm.Name)
				pod.UID = uid
				pod.Spec.NodeName = node
				pod.Status.Phase = corev1.PodRunning
				pod.Labels[annotations.InhibitNodeShutdownLabel] = ""
				return pod
			}

			sourcePod := newRunningPod(name+"-source", sourceNode, "source-uid")
			targetPod := newRunningPod(name+"-target", targetNode, "target-uid")

			// A pod of a node that stopped reporting may sit in Unknown instead of Running.
			hungPod := newRunningPod(name+"-hung", sourceNode, "hung-uid")
			hungPod.Status.Phase = corev1.PodUnknown

			fakeClient, _, vmState = setupEnvironment(vm, kvvm, kvvmi, sourcePod, targetPod, hungPod)
			h := NewSyncMetadataHandler(fakeClient)
			_, err := h.Handle(ctx, vmState)
			Expect(err).NotTo(HaveOccurred())

			err = fakeClient.Get(ctx, client.ObjectKeyFromObject(sourcePod), sourcePod)
			Expect(err).NotTo(HaveOccurred())
			Expect(sourcePod.GetLabels()).NotTo(HaveKey(annotations.InhibitNodeShutdownLabel))
			Expect(sourcePod.GetLabels()).To(HaveKeyWithValue(virtv1.VirtualMachineNameLabel, vm.Name))

			err = fakeClient.Get(ctx, client.ObjectKeyFromObject(hungPod), hungPod)
			Expect(err).NotTo(HaveOccurred())
			Expect(hungPod.GetLabels()).NotTo(HaveKey(annotations.InhibitNodeShutdownLabel))

			err = fakeClient.Get(ctx, client.ObjectKeyFromObject(targetPod), targetPod)
			Expect(err).NotTo(HaveOccurred())
			Expect(targetPod.GetLabels()).To(HaveKeyWithValue(annotations.InhibitNodeShutdownLabel, ""))
		})
	})
})

var _ = Describe("SyncMetadataHandler and the migration node affinity terms", func() {
	const (
		name      = "vm-migration-terms"
		namespace = "default"
	)

	// The feature gate that lets the volumes travel is locked to the edition, so only the branch of
	// an edition without volume migration is reachable here. It is the one that has to clean up:
	// an annotation left behind would make virt-controller answer as if the disks could travel.
	It("removes the annotation where the volumes cannot travel", func() {
		Expect(featuregates.Default().Enabled(featuregates.VolumeMigration)).To(BeFalse(),
			"the test relies on the edition of the build, not on a mutable gate")

		vm := vmbuilder.NewEmpty(name, namespace)
		kvvm := newEmptyKVVM(name, namespace)
		kvvm.Spec = virtv1.VirtualMachineSpec{
			Template: &virtv1.VirtualMachineInstanceTemplateSpec{ObjectMeta: metav1.ObjectMeta{}},
		}
		kvvm.Annotations = map[string]string{
			annotations.AnnMigrationNodeAffinityTerms: "[]",
			"user.example.com/keep":                   "kept",
		}

		fakeClient, _, vmState := setupEnvironment(vm, kvvm)
		_, err := NewSyncMetadataHandler(fakeClient).Handle(
			testutil.ContextBackgroundWithNoOpLogger(), vmState)
		Expect(err).NotTo(HaveOccurred())

		updated := &virtv1.VirtualMachine{}
		Expect(fakeClient.Get(context.Background(),
			client.ObjectKey{Namespace: namespace, Name: name}, updated)).To(Succeed())
		Expect(updated.Annotations).NotTo(HaveKey(annotations.AnnMigrationNodeAffinityTerms))
		Expect(updated.Annotations).To(HaveKeyWithValue("user.example.com/keep", "kept"),
			"only the one annotation is taken away")
	})
})

var _ = Describe("SyncMetadataHandler.updateKVVMSpecTemplateMetadataAnnotations", func() {
	h := &SyncMetadataHandler{}

	It("keeps kvbuilder-owned template annotations while propagating VM annotations", func() {
		curr := map[string]string{
			annotations.AnnSchedulerExtraPVCs: "pvc-a,pvc-b",
			"user.example.com/old":            "gone",
		}
		newAnno := map[string]string{
			"user.example.com/new": "propagated",
		}

		res := h.updateKVVMSpecTemplateMetadataAnnotations(curr, newAnno)

		Expect(res).To(HaveKeyWithValue(annotations.AnnSchedulerExtraPVCs, "pvc-a,pvc-b"))
		Expect(res).To(HaveKeyWithValue("user.example.com/new", "propagated"))
		Expect(res).NotTo(HaveKey("user.example.com/old"))
	})

	// Mirroring it would change the rendered template on every reconcile that follows the annotation,
	// and the rendering would take it back out again.
	It("keeps the migration node affinity terms out of the template", func() {
		res := h.updateKVVMSpecTemplateMetadataAnnotations(nil, map[string]string{
			annotations.AnnMigrationNodeAffinityTerms: "[]",
			"user.example.com/new":                    "propagated",
		})

		Expect(res).NotTo(HaveKey(annotations.AnnMigrationNodeAffinityTerms))
		Expect(res).To(HaveKeyWithValue("user.example.com/new", "propagated"))
	})
})

// The feature gate that lets the volumes travel is locked to the edition, so the value of the
// annotation is checked on its own rather than through the handler.
var _ = Describe("MigrationNodeAffinityTerms", func() {
	term := func(key string, values ...string) corev1.NodeSelectorTerm {
		return corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key:      key,
				Operator: corev1.NodeSelectorOpIn,
				Values:   values,
			}},
		}
	}

	vmWithAffinity := func(terms ...corev1.NodeSelectorTerm) *v1alpha2.VirtualMachine {
		vm := &v1alpha2.VirtualMachine{}
		if len(terms) > 0 {
			vm.Spec.Affinity = &v1alpha2.VMAffinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: terms,
					},
				},
			}
		}
		return vm
	}

	It("renders an empty array for a machine without rules of its own", func() {
		value, err := MigrationNodeAffinityTerms(&v1alpha2.VirtualMachine{}, &v1alpha2.VirtualMachineClass{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(value).To(Equal("[]"), "an empty array and a missing annotation must not read the same")
	})

	It("renders the rules of the machine", func() {
		value, err := MigrationNodeAffinityTerms(
			vmWithAffinity(term("zone", "a")), &v1alpha2.VirtualMachineClass{}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(value).To(ContainSubstring(`"key":"zone"`))
	})

	It("adds the node selector of the class", func() {
		class := &v1alpha2.VirtualMachineClass{Spec: v1alpha2.VirtualMachineClassSpec{
			NodeSelector: v1alpha2.NodeSelector{
				MatchExpressions: []corev1.NodeSelectorRequirement{{
					Key:      "cpu",
					Operator: corev1.NodeSelectorOpExists,
				}},
			},
		}}
		value, err := MigrationNodeAffinityTerms(vmWithAffinity(term("zone", "a")), class, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(value).To(ContainSubstring(`"key":"zone"`))
		Expect(value).To(ContainSubstring(`"key":"cpu"`))
	})

	// The volumes that stay behind keep the machine on their node, so they belong to the rules that
	// hold after the migration just as much as the ones its owner wrote.
	It("narrows the rules by the volumes that stay", func() {
		value, err := MigrationNodeAffinityTerms(
			vmWithAffinity(term("zone", "a")),
			&v1alpha2.VirtualMachineClass{},
			[]corev1.NodeSelectorTerm{term("topology.local/node", "node-1")},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(value).To(ContainSubstring(`"key":"zone"`))
		Expect(value).To(ContainSubstring(`"key":"topology.local/node"`))
	})

	It("renders the volumes that stay alone when the machine has no rules of its own", func() {
		value, err := MigrationNodeAffinityTerms(
			&v1alpha2.VirtualMachine{},
			&v1alpha2.VirtualMachineClass{},
			[]corev1.NodeSelectorTerm{term("topology.local/node", "node-1")},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(value).To(ContainSubstring(`"key":"topology.local/node"`))
	})
})
