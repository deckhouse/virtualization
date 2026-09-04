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

	sizingpolicy "github.com/deckhouse/virtualization-controller/pkg/common/sizing_policy"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/promutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

type dataMetric struct {
	Name                                string
	Namespace                           string
	Node                                string
	UID                                 string
	Phase                               v1alpha2.MachinePhase
	CPUConfigurationCores               float64
	CPUConfigurationCoreFraction        float64
	CPUCores                            float64
	CPUCoreFraction                     float64
	CPURuntimeOverhead                  float64
	MemoryConfigurationSize             float64
	MemoryRuntimeOverhead               float64
	AwaitingRestartToApplyConfiguration bool
	ConfigurationApplied                bool
	AgentReady                          bool
	RunPolicy                           v1alpha2.RunPolicy
	Pods                                []v1alpha2.VirtualMachinePod
	Labels                              map[string]string
	Annotations                         map[string]string
	firmwareUpToDate                    bool
	// AppliedVirtualMachineClassName is the class name that is actually applied to the running VM.
	// It may differ from spec.virtualMachineClassName if the spec was changed but the VM wasn't restarted.
	AppliedVirtualMachineClassName string
	Migratable                     bool
	// EvictionRequiredReason is empty while the node running the virtual machine works as usual.
	EvictionRequiredReason string
	// MigratableKnown is false while the virtual machine is not running: migratability is not
	// evaluated then, and the metric is not exported instead of reporting a stale value.
	MigratableKnown bool
	// MigratableReason tells the answers of the same value apart: a machine whose disks travel
	// along with it and a machine with no node to take it right now are both migratable.
	MigratableReason string
	// Migration is nil for a machine that has never migrated, and no migration series is
	// exported for it.
	Migration *migrationDataMetric
}

// migrationDataMetric describes the last known migration of the virtual machine.
type migrationDataMetric struct {
	SourceNode string
	TargetNode string
	Result     string
	// Start and End are unix seconds; End stays 0 while the migration is in progress.
	Start float64
	End   float64
	// VolumeMigration is true when the disks move to another storage along with the memory.
	VolumeMigration bool
}

// migrationResultInProgress stands for the empty result the API reports while the machine is still
// travelling: an empty label value is indistinguishable from a missing label in PromQL.
const migrationResultInProgress = "InProgress"

// DO NOT mutate VirtualMachine!
func newDataMetric(vm *v1alpha2.VirtualMachine) *dataMetric {
	if vm == nil {
		return nil
	}
	res := vm.Status.Resources
	cf := getPercent(res.CPU.CoreFraction)
	// spec.cpu.coreFraction may be the literal "Auto", which getPercent cannot parse and
	// would report as 0. Report the value the autoscaler drives
	// (status.recommendedResources.cpu.coreFraction) as the effective configured fraction instead.
	specCoreFraction := vm.Spec.CPU.CoreFraction
	if specCoreFraction == v1alpha2.CoreFractionAuto {
		specCoreFraction = sizingpolicy.RecommendedCoreFraction(vm)
	}
	cfSpec := getPercent(specCoreFraction)

	var (
		awaitingRestartToApplyConfiguration bool
		configurationApplied                bool
		agentReady                          bool
		firmwareUpToDate                    bool
	)

	awaitingRestartToApplyConfigurationCondition, _ := conditions.GetCondition(vmcondition.TypeAwaitingRestartToApplyConfiguration, vm.Status.Conditions)
	awaitingRestartToApplyConfiguration = awaitingRestartToApplyConfigurationCondition.Status == metav1.ConditionTrue

	configurationAppliedCondition, _ := conditions.GetCondition(vmcondition.TypeConfigurationApplied, vm.Status.Conditions)
	configurationApplied = configurationAppliedCondition.Status != metav1.ConditionFalse

	agentReadyCondition, _ := conditions.GetCondition(vmcondition.TypeAgentReady, vm.Status.Conditions)
	agentReady = agentReadyCondition.Status == metav1.ConditionTrue

	firmwareUpToDateCondition, _ := conditions.GetCondition(vmcondition.TypeFirmwareUpToDate, vm.Status.Conditions)
	firmwareUpToDate = firmwareUpToDateCondition.Status != metav1.ConditionFalse

	// A machine with local disks is migratable in EE — its volumes travel along with it — and the
	// condition says so, which is why the condition is the source here rather than the disks.
	migratableCondition, hasMigratableCondition := conditions.GetCondition(vmcondition.TypeMigratable, vm.Status.Conditions)

	pods := make([]v1alpha2.VirtualMachinePod, len(vm.Status.VirtualMachinePods))
	for i, pod := range vm.Status.VirtualMachinePods {
		pods[i] = *pod.DeepCopy()
	}

	return &dataMetric{
		Name:                                vm.Name,
		Namespace:                           vm.Namespace,
		Node:                                vm.Status.Node,
		UID:                                 string(vm.UID),
		Phase:                               vm.Status.Phase,
		CPUConfigurationCores:               float64(vm.Spec.CPU.Cores),
		CPUConfigurationCoreFraction:        float64(cfSpec.IntValue()),
		CPUCores:                            float64(res.CPU.Cores),
		CPUCoreFraction:                     float64(cf.IntValue()),
		CPURuntimeOverhead:                  float64(res.CPU.RuntimeOverhead.MilliValue()),
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
		firmwareUpToDate:       firmwareUpToDate,
		Migratable:             migratableCondition.Status == metav1.ConditionTrue,
		MigratableKnown:        hasMigratableCondition,
		MigratableReason:       migratableCondition.Reason,
		EvictionRequiredReason: evictionRequiredReason(vm),
		Migration:              newMigrationDataMetric(vm),
	}
}

// newMigrationDataMetric reports the last known migration of the virtual machine. The state stays
// in the status once the migration is over, so the metric answers "where did the machine go" long
// after the seconds the migration itself took, no matter whether a scrape fell into that window.
func newMigrationDataMetric(vm *v1alpha2.VirtualMachine) *migrationDataMetric {
	state := vm.Status.MigrationState
	if state == nil {
		return nil
	}

	m := &migrationDataMetric{
		SourceNode:      state.Source.Node,
		TargetNode:      state.Target.Node,
		Result:          string(state.Result),
		VolumeMigration: state.VolumeMigration,
	}
	if m.Result == "" {
		m.Result = migrationResultInProgress
	}
	if state.StartTimestamp != nil {
		m.Start = float64(state.StartTimestamp.Unix())
	}
	if state.EndTimestamp != nil {
		m.End = float64(state.EndTimestamp.Unix())
	}

	return m
}

// evictionRequiredReason reports what the platform is going to do with the virtual machine while
// its node is being taken out of service. It stays empty for a machine whose node works as usual,
// so no series is exported for it.
func evictionRequiredReason(vm *v1alpha2.VirtualMachine) string {
	cond, found := conditions.GetCondition(vmcondition.TypeEvictionRequired, vm.Status.Conditions)
	if !found || cond.Status != metav1.ConditionTrue {
		return ""
	}
	return cond.Reason
}

func getPercent(s string) intstr.IntOrString {
	return intstr.FromString(strings.TrimSuffix(s, "%"))
}
