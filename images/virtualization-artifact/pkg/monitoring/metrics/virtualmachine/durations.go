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

package virtualmachine

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
)

const (
	LaunchStageWaitingForDependencies = "waiting_for_dependencies"
	LaunchStageVirtualMachineStarting = "virtual_machine_starting"
	LaunchStageGuestOSAgentStarting   = "guest_os_agent_starting"
	LaunchStageTotal                  = "total"
)

const (
	MetricVirtualMachineLaunchDuration    = "virtualmachine_launch_duration_seconds"
	MetricVirtualMachineMigrationDuration = "virtualmachine_migration_duration_seconds"
	MetricVirtualMachineShutdownDuration  = "virtualmachine_shutdown_duration_seconds"
)

const (
	MigrationResultSucceeded = "succeeded"
	MigrationResultFailed    = "failed"
)

const (
	MigrationStageWaiting   = "waiting"
	MigrationStagePreparing = "preparing"
	MigrationStageTransfer  = "transfer"
	MigrationStageTotal     = "total"
)

const (
	MigrationTypeMigrate = "migrate"
	MigrationTypeEvict   = "evict"
)

// The phase transition timestamps are whole seconds, and a launch or a migration longer than an
// hour is broken rather than slow.
var durationBuckets = []float64{
	1, 2, 5, 10, 20, 30, 60, 120, 300, 600, 1800, 3600,
}

// No identity labels on the histograms: a series per bucket per label combination, so a machine
// name would multiply that by the size of the cluster. The identity lives in the gauges.
var LaunchDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: metrics.MetricNamespace,
	Name:      MetricVirtualMachineLaunchDuration,
	Help:      "The time a virtual machine spent in a launch stage.",
	Buckets:   durationBuckets,
}, []string{"stage"})

var MigrationDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: metrics.MetricNamespace,
	Name:      MetricVirtualMachineMigrationDuration,
	Help:      "The time a virtual machine migration spent in a stage, by the type of the operation that started it and by result.",
	Buckets:   durationBuckets,
}, []string{"type", "stage", "result"})

// Taken from the phase transitions rather than from an operation, so a guest that powers itself
// off is counted too.
var ShutdownDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
	Namespace: metrics.MetricNamespace,
	Name:      MetricVirtualMachineShutdownDuration,
	Help:      "The time a virtual machine took to stop, from Stopping to Stopped.",
	Buckets:   durationBuckets,
})

func ObserveShutdown(d time.Duration) {
	if d < 0 {
		return
	}
	ShutdownDuration.Observe(d.Seconds())
}

func ObserveMigration(migrationType, stage, result string, d time.Duration) {
	if d < 0 {
		return
	}
	MigrationDuration.WithLabelValues(migrationType, stage, result).Observe(d.Seconds())
}

func ObserveLaunchStage(stage string, d time.Duration) {
	if d < 0 {
		return
	}
	LaunchDuration.WithLabelValues(stage).Observe(d.Seconds())
}
