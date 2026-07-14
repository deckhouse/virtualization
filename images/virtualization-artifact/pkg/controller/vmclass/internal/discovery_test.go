/*
Copyright 2024 Flant JSC

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmclasscondition"
)

const controllerNamespace = "d8-virtualization"

func newNodeWithLabels(name string, labels map[string]string) *corev1.Node {
	return &corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
}

func newVirtHandlerPod(nodeName string) *corev1.Pod {
	return &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "virt-handler-" + nodeName,
			Namespace: controllerNamespace,
			Labels: map[string]string{
				virtv1.AppLabel: "virt-handler",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
		},
	}
}

func newVMClass(name string, cpuType v1alpha2.CPUType, nodeSelector *v1alpha2.NodeSelector, discoveryNodeSelector *metav1.LabelSelector) *v1alpha2.VirtualMachineClass {
	vmc := &v1alpha2.VirtualMachineClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "VirtualMachineClass",
			APIVersion: v1alpha2.Version,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha2.VirtualMachineClassSpec{
			CPU: v1alpha2.CPU{
				Type: cpuType,
			},
		},
	}
	if nodeSelector != nil {
		vmc.Spec.NodeSelector = *nodeSelector
	}
	if discoveryNodeSelector != nil {
		vmc.Spec.CPU.Discovery = &v1alpha2.CpuDiscovery{
			NodeSelector: *discoveryNodeSelector,
		}
	}
	return vmc
}

func setupDiscoveryEnvironment(vmc *v1alpha2.VirtualMachineClass, objs ...client.Object) (state.VirtualMachineClassState, *reconciler.Resource[*v1alpha2.VirtualMachineClass, v1alpha2.VirtualMachineClassStatus]) {
	GinkgoHelper()
	Expect(vmc).ToNot(BeNil())
	allObjects := []client.Object{vmc}
	allObjects = append(allObjects, objs...)

	fakeClient, err := testutil.NewFakeClientWithObjects(allObjects...)
	Expect(err).NotTo(HaveOccurred())

	resource := reconciler.NewResource(client.ObjectKeyFromObject(vmc), fakeClient,
		func() *v1alpha2.VirtualMachineClass {
			return &v1alpha2.VirtualMachineClass{}
		},
		func(obj *v1alpha2.VirtualMachineClass) v1alpha2.VirtualMachineClassStatus {
			return obj.Status
		})
	err = resource.Fetch(context.Background())
	Expect(err).NotTo(HaveOccurred())

	vmcState := state.New(fakeClient, controllerNamespace, resource)

	return vmcState, resource
}

type nodeNamesDiffTestParams struct {
	prev    []string
	current []string
	added   []string
	removed []string
}

var _ = DescribeTable(
	"DiscoveryHandler NodeNamesDiff Test",
	func(params nodeNamesDiffTestParams) {
		calculatedAdded, calculatedRemoved := NodeNamesDiff(params.prev, params.current)
		Expect(calculatedAdded).Should(Equal(params.added))
		Expect(calculatedRemoved).Should(Equal(params.removed))
	},
	Entry(
		"Should be no diff",
		nodeNamesDiffTestParams{
			prev: []string{
				"node1",
				"node2",
			},
			current: []string{
				"node1",
				"node2",
			},
			added:   []string{},
			removed: []string{},
		},
	),
	Entry(
		"Should be added node",
		nodeNamesDiffTestParams{
			prev: []string{
				"node1",
				"node2",
			},
			current: []string{
				"node1",
				"node2",
				"node3",
			},
			added: []string{
				"node3",
			},
			removed: []string{},
		},
	),
	Entry(
		"Should be removed node",
		nodeNamesDiffTestParams{
			prev: []string{
				"node1",
				"node2",
				"node3",
			},
			current: []string{
				"node1",
				"node2",
			},
			added: []string{},
			removed: []string{
				"node3",
			},
		},
	),
	Entry(
		"Should be added and removed node",
		nodeNamesDiffTestParams{
			prev: []string{
				"node1",
				"node2",
			},
			current: []string{
				"node2",
				"node3",
			},
			added: []string{
				"node3",
			},
			removed: []string{
				"node1",
			},
		},
	),
	Entry(
		"Should be multiple added and removed node",
		nodeNamesDiffTestParams{
			prev: []string{
				"node3",
				"node4",
				"node5",
			},
			current: []string{
				"node1",
				"node2",
				"node3",
			},
			added: []string{
				"node1",
				"node2",
			},
			removed: []string{
				"node4",
				"node5",
			},
		},
	),
)

type discoveryCommonFeaturesTestParams struct {
	nodes            []corev1.Node
	expectedFeatures []string
}

var _ = DescribeTable(
	"DiscoveryHandler discoveryCommonFeatures Test",
	func(params discoveryCommonFeaturesTestParams) {
		handler := &DiscoveryHandler{}
		result := handler.discoveryCommonFeatures(params.nodes)
		if len(params.expectedFeatures) == 0 {
			Expect(result).To(BeEmpty())
		} else {
			Expect(result).To(ConsistOf(params.expectedFeatures))
		}
	},
	Entry(
		"All nodes with same features",
		discoveryCommonFeaturesTestParams{
			nodes: []corev1.Node{
				*newNodeWithLabels("node1", map[string]string{
					virtv1.CPUFeatureLabel + "vmx": "true",
					virtv1.CPUFeatureLabel + "svm": "true",
					virtv1.CPUFeatureLabel + "lm":  "true",
				}),
				*newNodeWithLabels("node2", map[string]string{
					virtv1.CPUFeatureLabel + "vmx": "true",
					virtv1.CPUFeatureLabel + "svm": "true",
					virtv1.CPUFeatureLabel + "lm":  "true",
				}),
				*newNodeWithLabels("node3", map[string]string{
					virtv1.CPUFeatureLabel + "vmx": "true",
					virtv1.CPUFeatureLabel + "svm": "true",
					virtv1.CPUFeatureLabel + "lm":  "true",
				}),
			},
			expectedFeatures: []string{"vmx", "svm", "lm"},
		},
	),
	Entry(
		"Partially overlapping features",
		discoveryCommonFeaturesTestParams{
			nodes: []corev1.Node{
				*newNodeWithLabels("node1", map[string]string{
					virtv1.CPUFeatureLabel + "vmx":    "true",
					virtv1.CPUFeatureLabel + "svm":    "true",
					virtv1.CPUFeatureLabel + "lm":     "true",
					virtv1.CPUFeatureLabel + "sse4.1": "true",
				}),
				*newNodeWithLabels("node2", map[string]string{
					virtv1.CPUFeatureLabel + "vmx": "true",
					virtv1.CPUFeatureLabel + "svm": "true",
					virtv1.CPUFeatureLabel + "lm":  "true",
				}),
				*newNodeWithLabels("node3", map[string]string{
					virtv1.CPUFeatureLabel + "vmx": "true",
					virtv1.CPUFeatureLabel + "svm": "true",
					virtv1.CPUFeatureLabel + "lm":  "true",
					virtv1.CPUFeatureLabel + "avx": "true",
				}),
			},
			expectedFeatures: []string{"vmx", "svm", "lm"},
		},
	),
	Entry(
		"No common features",
		discoveryCommonFeaturesTestParams{
			nodes: []corev1.Node{
				*newNodeWithLabels("node1", map[string]string{
					virtv1.CPUFeatureLabel + "vmx": "true",
					virtv1.CPUFeatureLabel + "svm": "true",
				}),
				*newNodeWithLabels("node2", map[string]string{
					virtv1.CPUFeatureLabel + "lm":     "true",
					virtv1.CPUFeatureLabel + "sse4.1": "true",
				}),
				*newNodeWithLabels("node3", map[string]string{
					virtv1.CPUFeatureLabel + "avx":  "true",
					virtv1.CPUFeatureLabel + "avx2": "true",
				}),
			},
			expectedFeatures: []string{},
		},
	),
	Entry(
		"Single node",
		discoveryCommonFeaturesTestParams{
			nodes: []corev1.Node{
				*newNodeWithLabels("node1", map[string]string{
					virtv1.CPUFeatureLabel + "vmx": "true",
					virtv1.CPUFeatureLabel + "svm": "true",
					virtv1.CPUFeatureLabel + "lm":  "true",
				}),
			},
			expectedFeatures: []string{"vmx", "svm", "lm"},
		},
	),
	Entry(
		"Empty node list",
		discoveryCommonFeaturesTestParams{
			nodes:            []corev1.Node{},
			expectedFeatures: []string{},
		},
	),
	Entry(
		"Nodes without CPU feature labels",
		discoveryCommonFeaturesTestParams{
			nodes: []corev1.Node{
				*newNodeWithLabels("node1", map[string]string{"some-label": "value"}),
				*newNodeWithLabels("node2", map[string]string{"other-label": "value"}),
				*newNodeWithLabels("node3", map[string]string{}),
			},
			expectedFeatures: []string{},
		},
	),
	Entry(
		"Mixed label values (not all true)",
		discoveryCommonFeaturesTestParams{
			nodes: []corev1.Node{
				*newNodeWithLabels("node1", map[string]string{
					virtv1.CPUFeatureLabel + "vmx": "true",
				}),
				*newNodeWithLabels("node2", map[string]string{
					virtv1.CPUFeatureLabel + "vmx": "false",
				}),
				*newNodeWithLabels("node3", map[string]string{
					virtv1.CPUFeatureLabel + "vmx": "true",
				}),
			},
			expectedFeatures: []string{},
		},
	),
)

var _ = Describe("DiscoveryHandler", func() {
	Context("Handle with various nodeSelector configurations", func() {
		It("should discover features from all virt-handler nodes when no nodeSelector is set", func() {
			node1 := newNodeWithLabels("node1", map[string]string{
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
			})
			node2 := newNodeWithLabels("node2", map[string]string{
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
			})
			node3 := newNodeWithLabels("node3", map[string]string{
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
			})

			virtHandler1 := newVirtHandlerPod("node1")
			virtHandler2 := newVirtHandlerPod("node2")
			virtHandler3 := newVirtHandlerPod("node3")

			vmc := newVMClass("test-no-selector", v1alpha2.CPUTypeDiscovery, nil, nil)

			vmcState, resource := setupDiscoveryEnvironment(vmc,
				node1, node2, node3,
				virtHandler1, virtHandler2, virtHandler3)

			ctx := context.Background()

			mockRecorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			handler := NewDiscoveryHandler(mockRecorder)

			_, err := handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed := resource.Changed()

			Expect(changed.Status.AvailableNodes).To(ConsistOf("node1", "node2", "node3"))
			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx", "svm"))

			cond := conditions.FindStatusCondition(changed.Status.Conditions, vmclasscondition.TypeDiscovered.String())
			Expect(cond).NotTo(BeNil())
			Expect(cond.Reason).To(Equal(vmclasscondition.ReasonDiscoverySucceeded.String()))
		})

		It("should derive features from discovery.nodeSelector.matchLabels pool", func() {
			node1 := newNodeWithLabels("node1", map[string]string{
				"node.deckhouse.io/group":      "worker",
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
			})
			node2 := newNodeWithLabels("node2", map[string]string{
				"node.deckhouse.io/group":      "worker",
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
			})
			node3 := newNodeWithLabels("node3", map[string]string{
				"node.deckhouse.io/group":      "master",
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
				virtv1.CPUFeatureLabel + "lm":  "true",
			})

			virtHandler1 := newVirtHandlerPod("node1")
			virtHandler2 := newVirtHandlerPod("node2")
			virtHandler3 := newVirtHandlerPod("node3")

			vmc := newVMClass("test-match-labels",
				v1alpha2.CPUTypeDiscovery,
				nil,
				&metav1.LabelSelector{
					MatchLabels: map[string]string{"node.deckhouse.io/group": "worker"},
				},
			)

			vmcState, resource := setupDiscoveryEnvironment(vmc,
				node1, node2, node3,
				virtHandler1, virtHandler2, virtHandler3)

			ctx := context.Background()

			mockRecorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			handler := NewDiscoveryHandler(mockRecorder)

			_, err := handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed := resource.Changed()

			// Features come from the discovery pool (worker nodes only), so lm
			// is excluded even though node3 has it.
			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx", "svm"))
			Expect(changed.Status.CpuFeatures.Enabled).NotTo(ContainElement("lm"))
			// spec.nodeSelector is empty, so every node exposing the discovered
			// features is schedulable, including node3 from outside the pool.
			Expect(changed.Status.AvailableNodes).To(ConsistOf("node1", "node2", "node3"))
		})

		It("should derive features from discovery.nodeSelector.matchExpressions pool", func() {
			node1 := newNodeWithLabels("node1", map[string]string{
				"node.deckhouse.io/group":      "worker",
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
			})
			node2 := newNodeWithLabels("node2", map[string]string{
				"node.deckhouse.io/group":      "compute",
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
			})
			node3 := newNodeWithLabels("node3", map[string]string{
				"node.deckhouse.io/group":      "master",
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
				virtv1.CPUFeatureLabel + "lm":  "true",
			})

			virtHandler1 := newVirtHandlerPod("node1")
			virtHandler2 := newVirtHandlerPod("node2")
			virtHandler3 := newVirtHandlerPod("node3")

			vmc := newVMClass("test-match-expressions",
				v1alpha2.CPUTypeDiscovery,
				nil,
				&metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "node.deckhouse.io/group",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"worker", "compute"},
						},
					},
				},
			)

			vmcState, resource := setupDiscoveryEnvironment(vmc,
				node1, node2, node3,
				virtHandler1, virtHandler2, virtHandler3)

			ctx := context.Background()

			mockRecorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			handler := NewDiscoveryHandler(mockRecorder)

			_, err := handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed := resource.Changed()

			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx", "svm"))
			Expect(changed.Status.CpuFeatures.Enabled).NotTo(ContainElement("lm"))
			// spec.nodeSelector is empty, so node3 also remains schedulable as it
			// exposes all discovered features.
			Expect(changed.Status.AvailableNodes).To(ConsistOf("node1", "node2", "node3"))
		})

		It("should keep discovery pool and schedulable nodes separate", func() {
			// discovery.nodeSelector narrows the feature-discovery pool, while
			// spec.nodeSelector narrows where VMs schedule. A node outside the
			// discovery pool but matching spec.nodeSelector and exposing the
			// discovered features must appear in availableNodes.
			node1 := newNodeWithLabels("node1", map[string]string{
				"node.deckhouse.io/group":      "worker",
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
			})
			node2 := newNodeWithLabels("node2", map[string]string{
				"node.deckhouse.io/group":      "compute",
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
			})

			virtHandler1 := newVirtHandlerPod("node1")
			virtHandler2 := newVirtHandlerPod("node2")

			// Discovery pool is worker only; spec.nodeSelector allows compute too.
			vmc := newVMClass("test-pool-separation",
				v1alpha2.CPUTypeDiscovery,
				&v1alpha2.NodeSelector{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      "node.deckhouse.io/group",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"worker", "compute"},
						},
					},
				},
				&metav1.LabelSelector{
					MatchLabels: map[string]string{"node.deckhouse.io/group": "worker"},
				},
			)

			vmcState, resource := setupDiscoveryEnvironment(vmc,
				node1, node2,
				virtHandler1, virtHandler2)

			ctx := context.Background()

			mockRecorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			handler := NewDiscoveryHandler(mockRecorder)

			_, err := handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed := resource.Changed()

			// Features discovered from the worker pool only.
			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx", "svm"))
			// Both nodes match spec.nodeSelector and expose the discovered
			// features, so both are schedulable even though node2 is outside the
			// discovery pool.
			Expect(changed.Status.AvailableNodes).To(ConsistOf("node1", "node2"))
		})

		It("should exclude schedulable nodes missing a discovered feature", func() {
			// A node matching spec.nodeSelector but lacking a feature present in
			// the discovery pool must not be schedulable, since the universal CPU
			// model would not run on it.
			node1 := newNodeWithLabels("node1", map[string]string{
				"kubernetes.io/hostname":       "node1",
				"node.deckhouse.io/group":      "worker",
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
			})
			node2 := newNodeWithLabels("node2", map[string]string{
				"node.deckhouse.io/group":      "worker",
				virtv1.CPUFeatureLabel + "vmx": "true",
			})

			virtHandler1 := newVirtHandlerPod("node1")
			virtHandler2 := newVirtHandlerPod("node2")

			// Discovery pool is node1 only; spec.nodeSelector is empty so both
			// nodes are candidates for scheduling.
			vmc := newVMClass("test-feature-mismatch",
				v1alpha2.CPUTypeDiscovery,
				nil,
				&metav1.LabelSelector{
					MatchLabels: map[string]string{"kubernetes.io/hostname": "node1"},
				},
			)

			vmcState, resource := setupDiscoveryEnvironment(vmc,
				node1, node2,
				virtHandler1, virtHandler2)

			ctx := context.Background()

			mockRecorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			handler := NewDiscoveryHandler(mockRecorder)

			_, err := handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed := resource.Changed()

			// Discovered model is derived from node1 only.
			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx", "svm"))
			// node2 misses svm, so it cannot run the universal CPU model and is
			// excluded from availableNodes even though it matches spec.nodeSelector.
			Expect(changed.Status.AvailableNodes).To(ConsistOf("node1"))
		})

		It("should keep discovered features pinned when node composition changes", func() {
			// The discovered feature set is the CPU model of already running
			// VMs: once discovered it must never change. Node composition
			// changes are reflected only in availableNodes, which is
			// recomputed against the pinned model each reconcile.
			nodeOld := newNodeWithLabels("node-old", map[string]string{
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "avx": "true",
				virtv1.CPUFeatureLabel + "hle": "true",
			})
			nodeNew := newNodeWithLabels("node-new", map[string]string{
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "avx": "true",
				virtv1.CPUFeatureLabel + "fma": "true",
			})
			handlerOld := newVirtHandlerPod("node-old")
			handlerNew := newVirtHandlerPod("node-new")

			vmc := newVMClass("test-pinned-model", v1alpha2.CPUTypeDiscovery, nil, nil)
			vmcState, resource := setupDiscoveryEnvironment(vmc,
				nodeOld, nodeNew,
				handlerOld, handlerNew)

			ctx := context.Background()
			mockRecorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			handler := NewDiscoveryHandler(mockRecorder)

			// Two passes are required: first sets the Discovered condition to
			// Unknown (addAllUnknown requeue), second performs the actual
			// discovery and writes Status.CpuFeatures.
			_, err := handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed := resource.Changed()
			Expect(changed.Status.AvailableNodes).To(ConsistOf("node-old", "node-new"))
			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx", "avx"))

			// Simulate node-old disappearing (e.g. drained, deleted). The
			// class carries the previously discovered status, as it would
			// after the status update was persisted.
			vmc.Status.CpuFeatures.Enabled = changed.Status.CpuFeatures.Enabled
			vmcState, resource = setupDiscoveryEnvironment(vmc,
				nodeNew,
				handlerNew)
			// Replay the same reconcile pattern again on the fresh state.
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed = resource.Changed()
			// The model is pinned: fma from node-new must not be added.
			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx", "avx"))
			// node-new exposes every pinned feature, so it stays schedulable.
			Expect(changed.Status.AvailableNodes).To(ConsistOf("node-new"))
		})

		It("should report empty availableNodes when no node exposes the pinned features", func() {
			// After the pool that produced the model is gone, a node lacking a
			// pinned feature must not become schedulable, and the class must
			// honestly report an empty availableNodes instead of weakening the
			// model.
			nodeNew := newNodeWithLabels("node-new", map[string]string{
				virtv1.CPUFeatureLabel + "vmx": "true",
			})
			handlerNew := newVirtHandlerPod("node-new")

			vmc := newVMClass("test-pinned-unschedulable", v1alpha2.CPUTypeDiscovery, nil, nil)
			vmc.Status.CpuFeatures.Enabled = []string{"vmx", "avx"}
			vmcState, resource := setupDiscoveryEnvironment(vmc, nodeNew, handlerNew)

			ctx := context.Background()
			mockRecorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			handler := NewDiscoveryHandler(mockRecorder)

			_, err := handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed := resource.Changed()
			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx", "avx"))
			Expect(changed.Status.AvailableNodes).To(BeEmpty())

			// The model exists and is in use — Discovered stays True even
			// though nothing is currently schedulable.
			cond := conditions.FindStatusCondition(changed.Status.Conditions, vmclasscondition.TypeDiscovered.String())
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should mark Discovered=False when available nodes have no common CPU features", func() {
			// Pick two labels that exist on opposite nodes so there is no
			// common CPU feature between availableNodes.
			node1 := newNodeWithLabels("node1", map[string]string{
				virtv1.CPUFeatureLabel + "hle": "true",
			})
			node2 := newNodeWithLabels("node2", map[string]string{
				virtv1.CPUFeatureLabel + "rtm": "true",
			})
			handler1 := newVirtHandlerPod("node1")
			handler2 := newVirtHandlerPod("node2")

			vmc := newVMClass("test-no-common", v1alpha2.CPUTypeDiscovery, nil, nil)
			vmcState, resource := setupDiscoveryEnvironment(vmc,
				node1, node2,
				handler1, handler2)

			ctx := context.Background()
			mockRecorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			handler := NewDiscoveryHandler(mockRecorder)

			_, err := handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed := resource.Changed()
			Expect(changed.Status.AvailableNodes).To(ConsistOf("node1", "node2"))
			Expect(changed.Status.CpuFeatures.Enabled).To(BeEmpty())

			cond := conditions.FindStatusCondition(changed.Status.Conditions, vmclasscondition.TypeDiscovered.String())
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(vmclasscondition.ReasonDiscoveryFailed.String()))
		})

		It("should populate enabled from spec.cpu.features and keep all matching nodes for Features type", func() {
			// UF1: Features type takes enabled features verbatim from spec; no
			// discovery happens and every node exposing the requested features is
			// schedulable.
			node1 := newNodeWithLabels("node1", map[string]string{
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
			})
			node2 := newNodeWithLabels("node2", map[string]string{
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
			})
			node3 := newNodeWithLabels("node3", map[string]string{
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
			})

			virtHandler1 := newVirtHandlerPod("node1")
			virtHandler2 := newVirtHandlerPod("node2")
			virtHandler3 := newVirtHandlerPod("node3")

			vmc := newVMClass("test-features-basic", v1alpha2.CPUTypeFeatures, nil, nil)
			vmc.Spec.CPU.Features = []string{"vmx", "svm"}

			vmcState, resource := setupDiscoveryEnvironment(vmc,
				node1, node2, node3,
				virtHandler1, virtHandler2, virtHandler3)

			ctx := context.Background()

			mockRecorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			handler := NewDiscoveryHandler(mockRecorder)

			_, err := handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed := resource.Changed()

			Expect(changed.Status.AvailableNodes).To(ConsistOf("node1", "node2", "node3"))
			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx", "svm"))
			Expect(changed.Status.CpuFeatures.NotEnabledCommon).To(BeEmpty())

			cond := conditions.FindStatusCondition(changed.Status.Conditions, vmclasscondition.TypeDiscovered.String())
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(vmclasscondition.ReasonDiscoverySkip.String()))
		})

		It("should exclude nodes missing a requested feature for Features type", func() {
			// UF2: Nodes() filters via matchLabels on every spec feature, so a
			// node lacking any requested feature never reaches availableNodes.
			node1 := newNodeWithLabels("node1", map[string]string{
				virtv1.CPUFeatureLabel + "vmx": "true",
				virtv1.CPUFeatureLabel + "svm": "true",
			})
			node2 := newNodeWithLabels("node2", map[string]string{
				virtv1.CPUFeatureLabel + "vmx": "true",
			})

			virtHandler1 := newVirtHandlerPod("node1")
			virtHandler2 := newVirtHandlerPod("node2")

			vmc := newVMClass("test-features-missing", v1alpha2.CPUTypeFeatures, nil, nil)
			vmc.Spec.CPU.Features = []string{"vmx", "svm"}

			vmcState, resource := setupDiscoveryEnvironment(vmc,
				node1, node2,
				virtHandler1, virtHandler2)

			ctx := context.Background()

			mockRecorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			handler := NewDiscoveryHandler(mockRecorder)

			_, err := handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed := resource.Changed()

			Expect(changed.Status.AvailableNodes).To(ConsistOf("node1"))
			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx", "svm"))
			Expect(changed.Status.CpuFeatures.NotEnabledCommon).To(BeEmpty())
		})

		It("should narrow availableNodes by spec.nodeSelector for Features type", func() {
			// UF3: spec.nodeSelector restricts scheduling independently of the
			// feature match performed by Nodes().
			node1 := newNodeWithLabels("node1", map[string]string{
				"node.deckhouse.io/group":      "worker",
				virtv1.CPUFeatureLabel + "vmx": "true",
			})
			node2 := newNodeWithLabels("node2", map[string]string{
				"node.deckhouse.io/group":      "worker",
				virtv1.CPUFeatureLabel + "vmx": "true",
			})
			node3 := newNodeWithLabels("node3", map[string]string{
				"node.deckhouse.io/group":      "master",
				virtv1.CPUFeatureLabel + "vmx": "true",
			})

			virtHandler1 := newVirtHandlerPod("node1")
			virtHandler2 := newVirtHandlerPod("node2")
			virtHandler3 := newVirtHandlerPod("node3")

			vmc := newVMClass("test-features-nodeselector", v1alpha2.CPUTypeFeatures, &v1alpha2.NodeSelector{
				MatchExpressions: []corev1.NodeSelectorRequirement{
					{
						Key:      "node.deckhouse.io/group",
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"worker"},
					},
				},
			}, nil)
			vmc.Spec.CPU.Features = []string{"vmx"}

			vmcState, resource := setupDiscoveryEnvironment(vmc,
				node1, node2, node3,
				virtHandler1, virtHandler2, virtHandler3)

			ctx := context.Background()

			mockRecorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			handler := NewDiscoveryHandler(mockRecorder)

			_, err := handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed := resource.Changed()

			Expect(changed.Status.AvailableNodes).To(ConsistOf("node1", "node2"))
			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx"))
			Expect(changed.Status.CpuFeatures.NotEnabledCommon).To(BeEmpty())
		})

		It("should report notEnabledCommon for Features type when nodes share extra features", func() {
			// UF4: notEnabledCommon (variant B) = commonFeatures(availableNodes)
			// minus spec.cpu.features. Every schedulable node supports sse2 but
			// the user did not request it, so it must be reported as not enabled.
			node1 := newNodeWithLabels("node1", map[string]string{
				virtv1.CPUFeatureLabel + "vmx":  "true",
				virtv1.CPUFeatureLabel + "sse2": "true",
			})
			node2 := newNodeWithLabels("node2", map[string]string{
				virtv1.CPUFeatureLabel + "vmx":  "true",
				virtv1.CPUFeatureLabel + "sse2": "true",
			})

			virtHandler1 := newVirtHandlerPod("node1")
			virtHandler2 := newVirtHandlerPod("node2")

			vmc := newVMClass("test-features-notenabled", v1alpha2.CPUTypeFeatures, nil, nil)
			vmc.Spec.CPU.Features = []string{"vmx"}

			vmcState, resource := setupDiscoveryEnvironment(vmc,
				node1, node2,
				virtHandler1, virtHandler2)

			ctx := context.Background()

			mockRecorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			handler := NewDiscoveryHandler(mockRecorder)

			_, err := handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed := resource.Changed()

			Expect(changed.Status.AvailableNodes).To(ConsistOf("node1", "node2"))
			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx"))
			Expect(changed.Status.CpuFeatures.NotEnabledCommon).To(ConsistOf("sse2"))
		})

		It("should leave notEnabledCommon empty for Discovery when availableNodes match the discovered model", func() {
			// UF5: for Discovery, availableNodes are filtered by nodesWithAllFeatures
			// against the discovered intersection, so commonFeatures(availableNodes)
			// collapses back to the enabled set and notEnabledCommon stays empty.
			// A non-empty result is only possible when spec.nodeSelector excludes
			// part of the discovery pool, shrinking the common set below the model.
			node1 := newNodeWithLabels("node1", map[string]string{
				virtv1.CPUFeatureLabel + "vmx":  "true",
				virtv1.CPUFeatureLabel + "avx":  "true",
				virtv1.CPUFeatureLabel + "sse2": "true",
			})
			node2 := newNodeWithLabels("node2", map[string]string{
				virtv1.CPUFeatureLabel + "vmx":  "true",
				virtv1.CPUFeatureLabel + "avx":  "true",
				virtv1.CPUFeatureLabel + "sse2": "true",
			})

			virtHandler1 := newVirtHandlerPod("node1")
			virtHandler2 := newVirtHandlerPod("node2")

			vmc := newVMClass("test-discovery-notenabled-empty", v1alpha2.CPUTypeDiscovery, nil, nil)

			vmcState, resource := setupDiscoveryEnvironment(vmc,
				node1, node2,
				virtHandler1, virtHandler2)

			ctx := context.Background()

			mockRecorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			handler := NewDiscoveryHandler(mockRecorder)

			_, err := handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed := resource.Changed()

			Expect(changed.Status.AvailableNodes).To(ConsistOf("node1", "node2"))
			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx", "avx", "sse2"))
			Expect(changed.Status.CpuFeatures.NotEnabledCommon).To(BeEmpty())
		})

		It("should report notEnabledCommon for Discovery when spec.nodeSelector shrinks availableNodes below the discovery pool", func() {
			// UF6: discovery pool is {node1,node2} with common features
			// {vmx,avx,sse2}, but spec.nodeSelector keeps only node1, whose labels
			// add clwb. commonFeatures(availableNodes={node1}) = {vmx,avx,sse2,clwb},
			// so clwb is common to the (shrunk) available set yet not part of the
			// discovered model and must surface in notEnabledCommon.
			node1 := newNodeWithLabels("node1", map[string]string{
				"node.deckhouse.io/group":       "worker",
				virtv1.CPUFeatureLabel + "vmx":  "true",
				virtv1.CPUFeatureLabel + "avx":  "true",
				virtv1.CPUFeatureLabel + "sse2": "true",
				virtv1.CPUFeatureLabel + "clwb": "true",
			})
			node2 := newNodeWithLabels("node2", map[string]string{
				"node.deckhouse.io/group":       "compute",
				virtv1.CPUFeatureLabel + "vmx":  "true",
				virtv1.CPUFeatureLabel + "avx":  "true",
				virtv1.CPUFeatureLabel + "sse2": "true",
			})

			virtHandler1 := newVirtHandlerPod("node1")
			virtHandler2 := newVirtHandlerPod("node2")

			vmc := newVMClass("test-discovery-notenabled-shrink", v1alpha2.CPUTypeDiscovery, &v1alpha2.NodeSelector{
				MatchExpressions: []corev1.NodeSelectorRequirement{
					{
						Key:      "node.deckhouse.io/group",
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"worker"},
					},
				},
			}, &metav1.LabelSelector{})

			vmcState, resource := setupDiscoveryEnvironment(vmc,
				node1, node2,
				virtHandler1, virtHandler2)

			ctx := context.Background()

			mockRecorder := &eventrecord.EventRecorderLoggerMock{
				EventfFunc: func(involved client.Object, eventtype, reason, messageFmt string, args ...any) {},
			}
			handler := NewDiscoveryHandler(mockRecorder)

			_, err := handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())
			_, err = handler.Handle(ctx, vmcState)
			Expect(err).NotTo(HaveOccurred())

			changed := resource.Changed()

			Expect(changed.Status.AvailableNodes).To(ConsistOf("node1"))
			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx", "avx", "sse2"))
			Expect(changed.Status.CpuFeatures.NotEnabledCommon).To(ConsistOf("clwb"))
		})
	})
})
