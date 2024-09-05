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

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type dataMetric struct {
	Name                                string
	Namespace                           string
	Node                                string
	UID                                 string
	Phase                               virtv2.MachinePhase
	CpuCores                            float64
	CpuCoreFraction                     float64
	CpuRequestedCores                   float64
	CpuRuntimeOverhead                  float64
	MemorySize                          float64
	MemoryRuntimeOverhead               float64
	AwaitingRestartToApplyConfiguration bool
	ConfigurationApplied                bool
	RunPolicy                           virtv2.RunPolicy
	Pods                                []virtv2.VirtualMachinePod
}

func newDataMetric(vm *virtv2.VirtualMachine) *dataMetric {
	if vm == nil {
		return nil
	}
	res := vm.Status.Resources
	cf := intstr.FromString(strings.TrimSuffix(res.CPU.CoreFraction, "%"))
	var (
		awaitingRestartToApplyConfiguration bool
		configurationApplied                bool
	)
	if cond, found := service.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration.String(),
		vm.Status.Conditions); found && cond.Status == metav1.ConditionTrue {
		awaitingRestartToApplyConfiguration = true
	}
	if cond, found := service.GetCondition(vmcondition.TypeConfigurationApplied.String(),
		vm.Status.Conditions); found && cond.Status == metav1.ConditionTrue {
		configurationApplied = true
	}
	return &dataMetric{
		Name:                                vm.Name,
		Namespace:                           vm.Namespace,
		Node:                                vm.Status.Node,
		UID:                                 string(vm.UID),
		Phase:                               vm.Status.Phase,
		CpuCores:                            float64(res.CPU.Cores),
		CpuCoreFraction:                     float64(cf.IntValue()),
		CpuRequestedCores:                   float64(res.CPU.RequestedCores.MilliValue()),
		CpuRuntimeOverhead:                  float64(res.CPU.RuntimeOverhead.MilliValue()),
		MemorySize:                          float64(res.Memory.Size.Value()),
		MemoryRuntimeOverhead:               float64(res.Memory.RuntimeOverhead.Value()),
		AwaitingRestartToApplyConfiguration: awaitingRestartToApplyConfiguration,
		ConfigurationApplied:                configurationApplied,
		RunPolicy:                           vm.Spec.RunPolicy,
		Pods:                                vm.Status.VirtualMachinePods,
	}
}
