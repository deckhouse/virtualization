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

package state

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/component-helpers/scheduling/corev1/nodeaffinity"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/array"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineClassState interface {
	VirtualMachineClass() *reconciler.Resource[*virtv2.VirtualMachineClass, virtv2.VirtualMachineClassStatus]
	VirtualMachines(ctx context.Context) ([]virtv2.VirtualMachine, error)
	Nodes(ctx context.Context) ([]corev1.Node, error)
	AvailableNodes(nodes []corev1.Node) ([]corev1.Node, error)
}

type state struct {
	controllerNamespace string
	client              client.Client
	vmClass             *reconciler.Resource[*virtv2.VirtualMachineClass, virtv2.VirtualMachineClassStatus]
}

func New(c client.Client, controllerNamespace string, vmClass *reconciler.Resource[*virtv2.VirtualMachineClass, virtv2.VirtualMachineClassStatus]) VirtualMachineClassState {
	return &state{client: c, controllerNamespace: controllerNamespace, vmClass: vmClass}
}

func (s *state) VirtualMachineClass() *reconciler.Resource[*virtv2.VirtualMachineClass, virtv2.VirtualMachineClassStatus] {
	return s.vmClass
}

func (s *state) VirtualMachines(ctx context.Context) ([]virtv2.VirtualMachine, error) {
	if s.vmClass == nil || s.vmClass.IsEmpty() {
		return nil, nil
	}
	name := s.vmClass.Current().GetName()
	vms := &virtv2.VirtualMachineList{}
	err := s.client.List(ctx, vms, client.MatchingFields{
		indexer.IndexFieldVMByClass: name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list virtual machines by vmclass %s: %w", name, err)
	}
	return vms.Items, nil
}

func nodeFilter(nodes []corev1.Node, filters ...array.FilterFunc[corev1.Node]) []corev1.Node {
	return array.Filter[corev1.Node](nodes, filters...)
}

func (s *state) Nodes(ctx context.Context) ([]corev1.Node, error) {
	if s.vmClass == nil || s.vmClass.IsEmpty() {
		return nil, nil
	}

	var (
		curr        = s.vmClass.Current()
		matchLabels map[string]string
	)
	virtHandlerNodes, err := s.getVirtHandlerNodeNames(ctx)
	if err != nil {
		return nil, err
	}
	filters := []array.FilterFunc[corev1.Node]{
		func(node *corev1.Node) (keep bool) {
			_, found := virtHandlerNodes[node.GetName()]
			return found
		},
	}

	switch curr.Spec.CPU.Type {
	case virtv2.CPUTypeHost, virtv2.CPUTypeHostPassthrough:
		// Node is always has the "Host" CPU type, no additional filters required.
	case virtv2.CPUTypeDiscovery:
		matchLabels = curr.Spec.CPU.Discovery.NodeSelector.MatchLabels
		filters = append(filters, func(node *corev1.Node) bool {
			return annotations.MatchExpressions(node.GetLabels(), curr.Spec.CPU.Discovery.NodeSelector.MatchExpressions)
		})
	case virtv2.CPUTypeModel:
		matchLabels = map[string]string{virtv1.CPUModelLabel + curr.Spec.CPU.Model: "true"}
	case virtv2.CPUTypeFeatures:
		ml := make(map[string]string, len(curr.Spec.CPU.Features))
		for _, feature := range curr.Spec.CPU.Features {
			ml[virtv1.CPUFeatureLabel+feature] = "true"
		}
		matchLabels = ml
	default:
		return nil, fmt.Errorf("unexpected cpu type %s", curr.Spec.CPU.Type)
	}
	nodes := &corev1.NodeList{}
	err = s.client.List(
		ctx,
		nodes,
		client.MatchingLabelsSelector{Selector: labels.SelectorFromSet(matchLabels)})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	return nodeFilter(nodes.Items, filters...), nil
}

func (s *state) getVirtHandlerNodeNames(ctx context.Context) (map[string]struct{}, error) {
	pods := &corev1.PodList{}
	err := s.client.List(ctx, pods, client.InNamespace(s.controllerNamespace),
		client.MatchingLabelsSelector{
			Selector: labels.SelectorFromSet(map[string]string{
				virtv1.AppLabel: "virt-handler",
			}),
		})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	nodes := make(map[string]struct{}, len(pods.Items))
	for _, pod := range pods.Items {
		nodes[pod.Spec.NodeName] = struct{}{}
	}
	return nodes, nil
}

func (s *state) AvailableNodes(nodes []corev1.Node) ([]corev1.Node, error) {
	if s.vmClass == nil || s.vmClass.IsEmpty() {
		return nil, nil
	}
	if len(nodes) == 0 {
		return nodes, nil
	}

	nodeSelector := s.vmClass.Current().Spec.NodeSelector

	filters := []array.FilterFunc[corev1.Node]{
		func(node *corev1.Node) bool {
			return annotations.MatchLabels(node.GetLabels(), nodeSelector.MatchLabels)
		},
	}

	if me := nodeSelector.MatchExpressions; len(me) > 0 {
		ns, err := nodeaffinity.NewNodeSelector(&corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: me}},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create NodeSelector: %w", err)
		}

		filters = append(filters, func(node *corev1.Node) bool {
			return ns.Match(node)
		})
	}
	return nodeFilter(nodes, filters...), nil
}
