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

package promutil

import (
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type dynamicMetric struct {
	desc   *prometheus.Desc
	metric *dto.Metric
}

func (m dynamicMetric) Desc() *prometheus.Desc {
	return m.desc
}

func (m dynamicMetric) Write(out *dto.Metric) error {
	out.Label = m.metric.Label
	out.Counter = m.metric.Counter
	out.Gauge = m.metric.Gauge
	out.Untyped = m.metric.Untyped
	return nil
}

func NewDynamicMetric(desc *prometheus.Desc, valueType prometheus.ValueType, value float64, labelValues []string, extraLabels prometheus.Labels) (prometheus.Metric, error) {
	metric := &dto.Metric{}
	if err := populateMetric(valueType, value, makeLabelPairs(desc, labelValues, extraLabels), nil, metric, nil); err != nil {
		return nil, err
	}

	return &dynamicMetric{
		desc:   desc,
		metric: metric,
	}, nil
}

func makeLabelPairs(desc *prometheus.Desc, labelValues []string, extraLabels prometheus.Labels) []*dto.LabelPair {
	pairs := prometheus.MakeLabelPairs(desc, labelValues)
	if extraLabels == nil {
		return pairs
	}
	for k, v := range extraLabels {
		pairs = append(pairs, &dto.LabelPair{
			Name:  proto.String(k),
			Value: proto.String(v),
		})
	}
	return pairs
}

func populateMetric(
	t prometheus.ValueType,
	v float64,
	labelPairs []*dto.LabelPair,
	e *dto.Exemplar,
	m *dto.Metric,
	ct *timestamppb.Timestamp,
) error {
	m.Label = labelPairs
	switch t {
	case prometheus.CounterValue:
		m.Counter = &dto.Counter{Value: proto.Float64(v), Exemplar: e, CreatedTimestamp: ct}
	case prometheus.GaugeValue:
		m.Gauge = &dto.Gauge{Value: proto.Float64(v)}
	case prometheus.UntypedValue:
		m.Untyped = &dto.Untyped{Value: proto.Float64(v)}
	default:
		return fmt.Errorf("encountered unknown type %v", t)
	}
	return nil
}
