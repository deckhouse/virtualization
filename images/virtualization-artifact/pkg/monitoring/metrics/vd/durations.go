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

package vd

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// totalProvisioning is called "provisioning" here: it is not the whole way to Ready, and DVCR
// provisioning is a part of it rather than another summand.
const (
	ProvisioningStageWaitingForDependencies  = "waiting_for_dependencies"
	ProvisioningStageWaitingForFirstConsumer = "waiting_for_first_consumer"
	ProvisioningStageDVCR                    = "dvcr_provisioning"
	ProvisioningStageProvisioning            = "provisioning"
)

const (
	DataSourceBlank          = "blank"
	DataSourceHTTP           = "http"
	DataSourceContainerImage = "container_image"
	DataSourceObjectRef      = "object_ref"
	DataSourceUpload         = "upload"
	DataSourceUnknown        = "unknown"
)

// A type this build does not know becomes unknown: copying the value from the spec unchecked would
// let the series count grow with the enum.
func DataSourceLabel(ds *v1alpha2.VirtualDiskDataSource) string {
	if ds == nil {
		return DataSourceBlank
	}

	switch ds.Type {
	case v1alpha2.DataSourceTypeHTTP:
		return DataSourceHTTP
	case v1alpha2.DataSourceTypeContainerImage:
		return DataSourceContainerImage
	case v1alpha2.DataSourceTypeObjectRef:
		return DataSourceObjectRef
	case v1alpha2.DataSourceTypeUpload:
		return DataSourceUpload
	default:
		return DataSourceUnknown
	}
}

const MetricDiskProvisioningDuration = "virtualdisk_provisioning_duration_seconds"

// The durations in the status are whole seconds. The tail reaches a day because two of the stages
// wait for a person: WaitForFirstConsumer until the machine is scheduled, an upload until someone
// uploads.
var provisioningBuckets = []float64{
	1, 2, 5, 10, 30, 60, 300, 900, 1800, 3600, 10800, 43200, 86400,
}

// No identity labels: a series per bucket per label combination, so a disk name would multiply
// that by the number of disks. The identity lives in the gauges.
var ProvisioningDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: metrics.MetricNamespace,
	Name:      MetricDiskProvisioningDuration,
	Help:      "The time a virtual disk spent in a provisioning stage.",
	Buckets:   provisioningBuckets,
}, []string{"stage", "datasource"})

func ObserveProvisioningStage(stage, datasource string, d time.Duration) {
	if d < 0 {
		return
	}
	ProvisioningDuration.WithLabelValues(stage, datasource).Observe(d.Seconds())
}
