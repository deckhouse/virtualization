/*
Copyright 2018 The CDI Authors.
Copyright 2025 Flant JSC

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

package pvcimporter

import (
	"github.com/machadovilaca/operator-observability/pkg/operatormetrics"
	ioprometheusclient "github.com/prometheus/client_model/go"
)

const (
	// ImportProgressMetricName is the name of the import progress metric
	ImportProgressMetricName = "kubevirt_cdi_import_progress_total"
)

var (
	importerMetrics = []operatormetrics.Metric{
		importProgress,
	}

	importProgress = operatormetrics.NewCounterVec(
		operatormetrics.MetricOpts{
			Name: ImportProgressMetricName,
			Help: "The import progress in percentage",
		},
		[]string{"ownerUID"},
	)
)

type ImportProgress struct {
	ownerUID string
}

func Progress(ownerUID string) *ImportProgress {
	return &ImportProgress{ownerUID}
}

// Add adds value to the importProgress metric
func (ip *ImportProgress) Add(value float64) {
	importProgress.WithLabelValues(ip.ownerUID).Add(value)
}

// Get returns the importProgress value
func (ip *ImportProgress) Get() (float64, error) {
	dto := &ioprometheusclient.Metric{}
	if err := importProgress.WithLabelValues(ip.ownerUID).Write(dto); err != nil {
		return 0, err
	}
	return dto.Counter.GetValue(), nil
}

// Delete removes the importProgress metric with the passed label
func (ip *ImportProgress) Delete() {
	importProgress.DeleteLabelValues(ip.ownerUID)
}
