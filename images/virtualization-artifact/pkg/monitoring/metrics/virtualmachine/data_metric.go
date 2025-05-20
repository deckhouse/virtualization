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

package virtualmachine

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/promutil"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type dataMetric struct {
	Name                                string
	Namespace                           string
	Node                                string
	UID                                 string
	Phase                               virtv2.MachinePhase
	CpuConfigurationCores               float64
	CpuConfigurationCoreFraction        float64
	CpuCores                            float64
	CpuCoreFraction                     float64
	CpuRuntimeOverhead                  float64
	MemoryConfigurationSize             float64
	MemoryRuntimeOverhead               float64
	AwaitingRestartToApplyConfiguration bool
	ConfigurationApplied                bool
	AgentReady                          bool
	RunPolicy                           virtv2.RunPolicy
	Pods                                []virtv2.VirtualMachinePod
	Labels                              map[string]string
	Annotations                         map[string]string
	firmwareUpToDate                    bool
}

// DO NOT mutate VirtualMachine!
func newDataMetric(vm *virtv2.VirtualMachine) *dataMetric {
	if vm == nil {
		return nil
	}
	res := vm.Status.Resources
	cf := getPercent(res.CPU.CoreFraction)
	cfSpec := getPercent(vm.Spec.CPU.CoreFraction)

	var (
		awaitingRestartToApplyConfiguration bool
		configurationApplied                bool
		agentReady                          bool
		firmwareUpToDate                    bool
	)

	awaitingRestartToApplyConfigurationCondition, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, vm.Status.Conditions)
	if awaitingRestartToApplyConfigurationCondition.Status == metav1.ConditionFalse {
		awaitingRestartToApplyConfiguration = true
	}

	configurationAppliedCondition, _ := conditions.GetCondition(vmcondition.TypeConfigurationApplied, vm.Status.Conditions)
	if configurationAppliedCondition.Status != metav1.ConditionFalse {
		configurationApplied = true
	}

	agentReadyCondition, _ := conditions.GetCondition(vmcondition.TypeAgentReady, vm.Status.Conditions)
	if agentReadyCondition.Status == metav1.ConditionTrue {
		agentReady = true
	}

	firmwareUpToDateCondition, _ := conditions.GetCondition(vmcondition.TypeFirmwareUpToDate, vm.Status.Conditions)
	if firmwareUpToDateCondition.Status != metav1.ConditionFalse {
		firmwareUpToDate = true
	}

	pods := make([]virtv2.VirtualMachinePod, len(vm.Status.VirtualMachinePods))
	for i, pod := range vm.Status.VirtualMachinePods {
		pods[i] = *pod.DeepCopy()
	}

	return &dataMetric{
		Name:                                vm.Name,
		Namespace:                           vm.Namespace,
		Node:                                vm.Status.Node,
		UID:                                 string(vm.UID),
		Phase:                               vm.Status.Phase,
		CpuConfigurationCores:               float64(vm.Spec.CPU.Cores),
		CpuConfigurationCoreFraction:        float64(cfSpec.IntValue()),
		CpuCores:                            float64(res.CPU.Cores),
		CpuCoreFraction:                     float64(cf.IntValue()),
		CpuRuntimeOverhead:                  float64(res.CPU.RuntimeOverhead.MilliValue()),
		MemoryConfigurationSize:             float64(vm.Spec.Memory.Size.Value()),
		MemoryRuntimeOverhead:               float64(res.Memory.RuntimeOverhead.Value()),
		AwaitingRestartToApplyConfiguration: awaitingRestartToApplyConfiguration,
		ConfigurationApplied:                configurationApplied,
		AgentReady:                          agentReady,
		RunPolicy:                           vm.Spec.RunPolicy,
		Pods:                                pods,
		Labels: promutil.WrapPrometheusLabels(vm.GetLabels(), "label", func(key, value string) bool {
			return false
		}),
		Annotations: promutil.WrapPrometheusLabels(vm.GetAnnotations(), "annotation", func(key, _ string) bool {
			return strings.HasPrefix(key, "kubectl.kubernetes.io")
		}),
		firmwareUpToDate: firmwareUpToDate,
	}
}

func getPercent(s string) intstr.IntOrString {
	return intstr.FromString(strings.TrimSuffix(s, "%"))
}
