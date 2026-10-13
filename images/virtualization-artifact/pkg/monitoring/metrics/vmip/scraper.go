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

package vmip

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/deckhouse/pkg/log"
)

func newScraper(ch chan<- prometheus.Metric, log *log.Logger) *scraper {
	return &scraper{ch: ch, log: log}
}

type scraper struct {
	ch     chan<- prometheus.Metric
	log    *log.Logger
	counts map[countKey]float64
}

type countKey struct {
	namespace string
	phase     string
}

func (s *scraper) Count(m *dataMetric) {
	if m == nil {
		return
	}
	if s.counts == nil {
		s.counts = make(map[countKey]float64)
	}
	s.counts[countKey{namespace: m.Namespace, phase: string(m.Phase)}]++
}

func (s *scraper) Report() {
	info := vmipMetrics[MetricDeprecatedVMIPCount]
	for key, count := range s.counts {
		metric, err := prometheus.NewConstMetric(info.Desc, prometheus.GaugeValue, count, key.namespace, key.phase)
		if err != nil {
			s.log.Warn(fmt.Sprintf("Error creating the new const dataMetric for %s: %s", info.Desc, err))
			continue
		}
		s.ch <- metric
	}
}
