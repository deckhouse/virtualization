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
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const collectorName = "vmip-collector"

func SetupCollector(reader client.Reader,
	registerer prometheus.Registerer,
	log *log.Logger,
) *Collector {
	c := &Collector{
		iterator: newUnsafeIterator(reader),
		reader:   reader,
		log:      log.With(logger.SlogCollector(collectorName)),
	}
	registerer.MustRegister(c)
	return c
}

type handler func(m *dataMetric) (stop bool)

type Iterator interface {
	Iter(ctx context.Context, h handler) error
}

type Collector struct {
	iterator Iterator
	reader   client.Reader
	log      *log.Logger
}

func (c Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range vmipMetrics {
		ch <- m.Desc
	}
}

// Collect counts the deprecated resources only where the sdn module serves the Main
// network: elsewhere there is nothing to migrate to, and the alert over the metric
// would fire on every cluster with a vmip.
func (c Collector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	mainIPAMAvailable, err := network.MainIPAMAvailable(ctx, c.reader)
	if err != nil {
		c.log.Error("Failed to check whether the sdn module serves the Main network", logger.SlogErr(err))
		return
	}
	if !mainIPAMAvailable {
		return
	}

	s := newScraper(ch, c.log)
	if err := c.iterator.Iter(ctx, func(m *dataMetric) (stop bool) {
		s.Count(m)
		return stop
	}); err != nil {
		c.log.Error("Failed to iterate over VirtualMachineIPAddresses", logger.SlogErr(err))
		return
	}
	s.Report()
}
