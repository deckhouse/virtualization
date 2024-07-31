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

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineClassState interface {
	VirtualMachineClass() *service.Resource[*virtv2.VirtualMachineClass, virtv2.VirtualMachineClassStatus]
	VirtualMachines(ctx context.Context) ([]virtv2.VirtualMachine, error)
	Nodes(ctx context.Context) ([]corev1.Node, error)
	NodesByVMNodeSelector(ctx context.Context) ([]corev1.Node, error)
}

type state struct {
	client  client.Client
	vmClass *service.Resource[*virtv2.VirtualMachineClass, virtv2.VirtualMachineClassStatus]
}

func New(c client.Client, vmClass *service.Resource[*virtv2.VirtualMachineClass, virtv2.VirtualMachineClassStatus]) VirtualMachineClassState {
	return &state{client: c, vmClass: vmClass}
}

func (s *state) VirtualMachineClass() *service.Resource[*virtv2.VirtualMachineClass, virtv2.VirtualMachineClassStatus] {
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

type filterFunc func([]corev1.Node) []corev1.Node

func (s *state) Nodes(ctx context.Context) ([]corev1.Node, error) {
	if s.vmClass == nil || s.vmClass.IsEmpty() {
		return nil, nil
	}
	curr := s.vmClass.Current()

	var matchLabels map[string]string
	var filter filterFunc

	switch curr.Spec.CPU.Type {
	case virtv2.CPUTypeHost, virtv2.CPUTypeHostPassthrough:
		return nil, nil
	case virtv2.CPUTypeDiscovery:
		matchLabels = curr.Spec.CPU.Discovery.MatchLabels
		filter = func(nodes []corev1.Node) []corev1.Node {
			var filtred []corev1.Node
			for _, node := range nodes {
				if common.MatchExpressions(node.GetLabels(), curr.Spec.CPU.Discovery.MatchExpressions) {
					filtred = append(filtred, node)
				}
			}
			return filtred
		}
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
	err := s.client.List(
		ctx,
		nodes,
		client.MatchingLabelsSelector{Selector: labels.SelectorFromSet(matchLabels)})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	result := nodes.Items
	if filter != nil {
		result = filter(result)
	}
	return result, nil
}

func (s *state) NodesByVMNodeSelector(ctx context.Context) ([]corev1.Node, error) {
	if s.vmClass == nil || s.vmClass.IsEmpty() {
		return nil, nil
	}
	nodeSelector := s.vmClass.Current().Spec.NodeSelector
	nodes := &corev1.NodeList{}
	err := s.client.List(
		ctx,
		nodes,
		client.MatchingLabelsSelector{Selector: labels.SelectorFromSet(nodeSelector.MatchLabels)})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}
	result := nodes.Items
	var filter filterFunc

	if len(nodeSelector.MatchExpressions) > 0 {
		ns, err := nodeaffinity.NewNodeSelector(&corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: nodeSelector.MatchExpressions}},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create NodeSelector: %w", err)
		}
		filter = func(nodes []corev1.Node) []corev1.Node {
			var filtered []corev1.Node
			for _, node := range nodes {
				if ns.Match(&node) {
					filtered = append(filtered, node)
				}
			}
			return filtered
		}
	}
	if filter != nil {
		result = filter(result)
	}
	return result, nil
}
