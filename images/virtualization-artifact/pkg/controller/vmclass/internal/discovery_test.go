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

		It("should filter nodes by discovery.nodeSelector.matchLabels", func() {
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

			Expect(changed.Status.CpuFeatures.Enabled).To(ConsistOf("vmx", "svm"))
			Expect(changed.Status.CpuFeatures.Enabled).NotTo(ContainElement("lm"))
		})

		It("should filter nodes by discovery.nodeSelector.matchExpressions with In operator", func() {
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
		})
	})
})
