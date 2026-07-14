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
	"fmt"
	"slices"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmclasscondition"
)

const nameDiscoveryHandler = "DiscoveryHandler"

func NewDiscoveryHandler(recorder eventrecord.EventRecorderLogger) *DiscoveryHandler {
	return &DiscoveryHandler{
		recorder: recorder,
	}
}

type DiscoveryHandler struct {
	recorder eventrecord.EventRecorderLogger
}

func (h *DiscoveryHandler) Handle(ctx context.Context, s state.VirtualMachineClassState) (reconcile.Result, error) {
	if s.VirtualMachineClass().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachineClass().Current()
	changed := s.VirtualMachineClass().Changed()

	if updated := addAllUnknown(changed, vmclasscondition.TypeDiscovered); updated {
		return reconcile.Result{Requeue: true}, nil
	}

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	cpuType := current.Spec.CPU.Type

	nodes, err := s.Nodes(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	availableNodes, err := s.AvailableNodes(nodes)
	if err != nil {
		return reconcile.Result{}, err
	}

	var featuresEnabled []string
	var discoveryPool []corev1.Node
	switch cpuType {
	case v1alpha2.CPUTypeDiscovery:
		// The discovered feature set is the CPU model of already running VMs:
		// once discovered it is pinned forever, so node composition changes
		// never mutate the model under running VMs. Only availableNodes is
		// recomputed each reconcile: schedulable nodes are restricted to those
		// that expose every pinned feature.
		if fs := current.Status.CpuFeatures.Enabled; len(fs) > 0 {
			featuresEnabled = fs
		} else {
			discoveryPool, err = s.DiscoveryNodes(ctx)
			if err != nil {
				return reconcile.Result{}, err
			}
			featuresEnabled = h.discoveryCommonFeatures(discoveryPool)
		}
		availableNodes = h.nodesWithAllFeatures(availableNodes, featuresEnabled)
	case v1alpha2.CPUTypeFeatures:
		featuresEnabled = current.Spec.CPU.Features
	}

	// notEnabledCommon: CPU features common to every available node but not
	// part of the enabled model. For Discovery these are features present on
	// all schedulable nodes beyond the discovered intersection; for Features
	// these are features the nodes support but the user did not request.
	var featuresNotEnabled []string
	if cpuType == v1alpha2.CPUTypeDiscovery || cpuType == v1alpha2.CPUTypeFeatures {
		commonFeatures := h.discoveryCommonFeatures(availableNodes)
		for _, cf := range commonFeatures {
			if !slices.Contains(featuresEnabled, cf) {
				featuresNotEnabled = append(featuresNotEnabled, cf)
			}
		}
	}

	availableNodeNames := make([]string, len(availableNodes))
	for i, n := range availableNodes {
		availableNodeNames[i] = n.GetName()
	}

	cb := conditions.NewConditionBuilder(vmclasscondition.TypeDiscovered).Generation(current.GetGeneration())
	switch cpuType {
	case v1alpha2.CPUTypeDiscovery:
		// A pinned model keeps Discovered=True even if the discovery pool is
		// currently empty: the model exists and is in use.
		if len(featuresEnabled) > 0 {
			cb.Message("").Reason(vmclasscondition.ReasonDiscoverySucceeded).Status(metav1.ConditionTrue)
			break
		}
		if len(discoveryPool) == 0 {
			cb.Message("No nodes match the discovery nodeSelector; skipping feature discovery.").
				Reason(vmclasscondition.ReasonDiscoveryFailed).
				Status(metav1.ConditionFalse)
			break
		}
		cb.Message("No common CPU features are discovered across discovery nodes.").
			Reason(vmclasscondition.ReasonDiscoveryFailed).
			Status(metav1.ConditionFalse)
	default:
		cb.Message(fmt.Sprintf("Discovery not needed for cpu.type %q", cpuType)).
			Reason(vmclasscondition.ReasonDiscoverySkip).
			Status(metav1.ConditionFalse)
	}
	conditions.SetCondition(cb, &changed.Status.Conditions)

	sort.Strings(availableNodeNames)
	sort.Strings(featuresEnabled)
	sort.Strings(featuresNotEnabled)

	addedNodes, removedNodes := NodeNamesDiff(current.Status.AvailableNodes, availableNodeNames)
	if len(addedNodes) > 0 || len(removedNodes) > 0 {
		if len(availableNodes) > 0 {
			h.recorder.Eventf(
				changed,
				corev1.EventTypeNormal,
				v1alpha2.ReasonVMClassNodesWereUpdated,
				"List of available nodes was updated, added nodes: %q, removed nodes: %q",
				addedNodes,
				removedNodes,
			)
		} else {
			h.recorder.Eventf(
				changed,
				corev1.EventTypeWarning,
				v1alpha2.ReasonVMClassAvailableNodesListEmpty,
				"List of available nodes was updated, now it's empty, removed nodes: %q",
				removedNodes,
			)
		}
	}

	changed.Status.AvailableNodes = availableNodeNames
	changed.Status.MaxAllocatableResources = h.maxAllocatableResources(availableNodes)
	changed.Status.CpuFeatures = v1alpha2.CpuFeatures{
		Enabled:          featuresEnabled,
		NotEnabledCommon: featuresNotEnabled,
	}

	return reconcile.Result{}, nil
}

func (h *DiscoveryHandler) Name() string {
	return nameDiscoveryHandler
}

func NodeNamesDiff(prev, current []string) (added, removed []string) {
	added = make([]string, 0)
	removed = make([]string, 0)
	prevMap := make(map[string]struct{})
	currentMap := make(map[string]struct{})

	for _, nodeName := range prev {
		prevMap[nodeName] = struct{}{}
	}

	for _, nodeName := range current {
		currentMap[nodeName] = struct{}{}
	}

	for _, nodeName := range prev {
		if _, ok := currentMap[nodeName]; !ok {
			removed = append(removed, nodeName)
		}
	}

	for _, nodeName := range current {
		if _, ok := prevMap[nodeName]; !ok {
			added = append(added, nodeName)
		}
	}

	return added, removed
}

func (h *DiscoveryHandler) discoveryCommonFeatures(nodes []corev1.Node) []string {
	if len(nodes) == 0 {
		return nil
	}
	featuresCount := make(map[string]int)
	for _, n := range nodes {
		for k, v := range n.GetLabels() {
			if strings.HasPrefix(k, virtv1.CPUFeatureLabel) && v == "true" {
				featuresCount[strings.TrimPrefix(k, virtv1.CPUFeatureLabel)]++
			}
		}
	}
	var features []string
	for k, v := range featuresCount {
		if v == len(nodes) {
			features = append(features, k)
		}
	}
	return features
}

// nodesWithAllFeatures keeps only nodes whose labels confirm every feature is
// present. When features is empty no filtering is applied and nodes are
// returned as-is.
func (h *DiscoveryHandler) nodesWithAllFeatures(nodes []corev1.Node, features []string) []corev1.Node {
	if len(features) == 0 {
		return nodes
	}
	required := make(map[string]struct{}, len(features))
	for _, f := range features {
		required[virtv1.CPUFeatureLabel+f] = struct{}{}
	}
	result := make([]corev1.Node, 0, len(nodes))
	for _, n := range nodes {
		labels := n.GetLabels()
		ok := true
		for k := range required {
			if labels[k] != "true" {
				ok = false
				break
			}
		}
		if ok {
			result = append(result, n)
		}
	}
	return result
}

func (h *DiscoveryHandler) maxAllocatableResources(nodes []corev1.Node) corev1.ResourceList {
	var (
		resourceList  corev1.ResourceList = make(map[corev1.ResourceName]resource.Quantity)
		resourceNames                     = []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory}
	)

	for _, node := range nodes {
		for _, resourceName := range resourceNames {
			newQ := node.Status.Allocatable[resourceName]
			if newQ.IsZero() {
				continue
			}
			oldQ := resourceList[resourceName]
			if newQ.Cmp(oldQ) == 1 {
				resourceList[resourceName] = newQ
			}
		}
	}
	return resourceList
}
