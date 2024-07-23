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
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmclass/internal/state"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmclasscondition"
)

const nameDiscoveryHandler = "DiscoveryHandler"

func NewDiscoveryHandler(client client.Client) *DiscoveryHandler {
	return &DiscoveryHandler{
		client: client,
	}
}

type DiscoveryHandler struct {
	client client.Client
}

func (h *DiscoveryHandler) Handle(ctx context.Context, s state.VirtualMachineClassState) (reconcile.Result, error) {
	if s.VirtualMachineClass().IsEmpty() {
		return reconcile.Result{}, nil
	}
	current := s.VirtualMachineClass().Current()
	changed := s.VirtualMachineClass().Changed()

	if updated := addAllUnknown(changed, vmclasscondition.TypeDiscovered.String()); updated {
		return reconcile.Result{Requeue: true}, nil
	}

	if isDeletion(current) {
		return reconcile.Result{}, nil
	}

	t := current.Spec.CPU.Type
	mgr := conditions.NewManager(changed.Status.Conditions)
	if t == virtv2.CPUTypeHostPassthrough || t == virtv2.CPUTypeHost {
		mgr.Update(conditions.NewConditionBuilder2(vmclasscondition.TypeDiscovered).
			Generation(current.GetGeneration()).
			Message(fmt.Sprintf("Discovery available only cpu.type %q", virtv2.CPUTypeDiscovery)).
			Reason2(vmclasscondition.ReasonDiscoverySkip).
			Status(metav1.ConditionFalse).Condition())
		changed.Status.Conditions = mgr.Generate()
		return reconcile.Result{}, nil
	}

	nodes, err := s.Nodes(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	availableNodes := make([]string, len(nodes))
	for i, n := range nodes {
		availableNodes[i] = n.GetName()
	}

	var featuresEnabled []string
	switch t {
	case virtv2.CPUTypeDiscovery:
		featuresEnabled = h.discoveryCommonFeatures(nodes)
	case virtv2.CPUTypeFeatures:
		featuresEnabled = current.Spec.CPU.Features
	}

	var featuresNotEnabled []string
	if t == virtv2.CPUTypeDiscovery || t == virtv2.CPUTypeFeatures {
		selectedNodes, err := s.NodesByVMNodeSelector(ctx)
		if err != nil {
			return reconcile.Result{}, err
		}
		commonFeatures := h.discoveryCommonFeatures(selectedNodes)
		for _, cf := range commonFeatures {
			if !slices.Contains(featuresEnabled, cf) {
				featuresNotEnabled = append(featuresNotEnabled, cf)
			}
		}
	}
	cb := conditions.NewConditionBuilder2(vmclasscondition.TypeDiscovered).Generation(current.GetGeneration())
	switch t {
	case virtv2.CPUTypeDiscovery:
		if len(featuresEnabled) > 0 {
			cb.Message("").Reason2(vmclasscondition.ReasonDiscoverySucceeded).Status(metav1.ConditionTrue)
			break
		}
		cb.Message("Detection nodes have no features.").
			Reason2(vmclasscondition.ReasonDiscoveryFailed).
			Status(metav1.ConditionFalse)
	default:
		cb.Message(fmt.Sprintf("Discovery available only cpu.type %q", virtv2.CPUTypeDiscovery)).
			Reason2(vmclasscondition.ReasonDiscoverySkip).
			Status(metav1.ConditionFalse)
	}

	mgr.Update(cb.Condition())
	changed.Status.Conditions = mgr.Generate()
	changed.Status.AvailableNodes = availableNodes
	changed.Status.CpuFeatures = virtv2.CpuFeatures{
		Enabled:          featuresEnabled,
		NotEnabledCommon: featuresNotEnabled,
	}

	return reconcile.Result{}, nil
}

func (h *DiscoveryHandler) Name() string {
	return nameDiscoveryHandler
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
