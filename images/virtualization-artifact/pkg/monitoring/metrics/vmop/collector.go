package vmop

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const collectorName = "vmop-collector"

func SetupCollector(reader client.Reader,
	registerer prometheus.Registerer,
	log *slog.Logger,
) *Collector {
	c := &Collector{
		iterator: newUnsafeIterator(reader),
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
	log      *slog.Logger
}

func (c Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, v := range vmopMetrics {
		ch <- v
	}
}

func (c Collector) Collect(ch chan<- prometheus.Metric) {
	s := newScraper(ch, c.log)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := c.iterator.Iter(ctx, func(m *dataMetric) (stop bool) {
		s.Report(m)
		return
	}); err != nil {
		c.log.Error("Failed to iterate of VMOPs", logger.SlogErr(err))
		return
	}
}
