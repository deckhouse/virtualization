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

package inplaceresize

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/component-base/featuregate"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
)

func New(featureGates featuregate.FeatureGate, client client.Client) *Service {
	return &Service{
		featureGates: featureGates,
		client:       client,
	}
}

type Service struct {
	featureGates featuregate.FeatureGate
	client       client.Client
}

func (s *Service) InProgress(kvvmi *virtv1.VirtualMachineInstance) bool {
	return s.featureGates.Enabled(featuregates.HotplugCPUAndMemoryWithInPlaceResize) && kvvmi != nil && kvvmi.GetAnnotations()[annotations.AnnVirtualMachineInstanceInPlaceResizeInProgress] == "true"
}

func (s *Service) IsCompleted(kvvmi *virtv1.VirtualMachineInstance) bool {
	cond, _ := conditions.GetKVVMICondition("PodResourceResizeInProgress", kvvmi.Status.Conditions)
	return cond.Reason == "PodResizeCompleted"
}

var ErrConditionNotFound = errors.New("condition not found")

func (s *Service) IsPossible(ctx context.Context, kvvmi *virtv1.VirtualMachineInstance) (bool, error) {
	cond, exists := conditions.GetKVVMICondition("PodResourceResizeInProgress", kvvmi.Status.Conditions)
	if !exists {
		return false, fmt.Errorf("failed to get PodResourceResizeInProgress condition: %w", ErrConditionNotFound)
	}

	switch cond.Reason {
	case "PodResizeCompleted":
		return false, nil
	case "PodResizePending", "PodResizeInProgress":
	default:
		return false, fmt.Errorf("unexpected PodResourceResizeInProgress condition reason: %s", cond.Reason)
	}

	pod, err := kvvm.FindPodByKVVMI(ctx, s.client, kvvmi)
	if err != nil {
		return false, err
	}

	podResizePending, _ := conditions.GetPodCondition(corev1.PodResizePending, pod.Status.Conditions)
	if podResizePending.Reason == corev1.PodReasonDeferred || podResizePending.Reason == corev1.PodReasonInfeasible {
		return false, nil
	}
	podResizeInProgress, _ := conditions.GetPodCondition(corev1.PodResizeInProgress, pod.Status.Conditions)
	if podResizeInProgress.Reason == corev1.PodReasonError {
		return false, nil
	}

	return true, nil
}

func (s *Service) ResizeCondition(kvvmi *virtv1.VirtualMachineInstance) virtv1.VirtualMachineInstanceCondition {
	cond, _ := conditions.GetKVVMICondition("PodResourceResizeInProgress", kvvmi.Status.Conditions)
	return cond
}

func (s *Service) CPUChange(kvvmi *virtv1.VirtualMachineInstance) bool {
	cond, _ := conditions.GetKVVMICondition(virtv1.VirtualMachineInstanceVCPUChange, kvvmi.Status.Conditions)
	return cond.Status == corev1.ConditionTrue
}

func (s *Service) MemoryChange(kvvmi *virtv1.VirtualMachineInstance) bool {
	cond, _ := conditions.GetKVVMICondition(virtv1.VirtualMachineInstanceMemoryChange, kvvmi.Status.Conditions)
	return cond.Status == corev1.ConditionTrue
}
