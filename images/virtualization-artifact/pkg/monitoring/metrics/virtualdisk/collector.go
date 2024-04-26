package virtualdisk

import (
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"

	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
	"github.com/deckhouse/virtualization-controller/pkg/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	MetricDiskStatusPhase = "virtualdisk_status_phase"
)

var diskMetrics = map[string]*prometheus.Desc{
	MetricDiskStatusPhase: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricDiskStatusPhase),
		"The virtualdisk current phase.",
		[]string{"name", "namespace", "uid", "phase"},
		nil),
}

func SetupCollector(lister Lister, registerer prometheus.Registerer) *Collector {
	c := &Collector{
		lister: lister,
	}

	registerer.MustRegister(c)
	return c
}

type Lister interface {
	List() ([]virtv2.VirtualDisk, error)
}

type Collector struct {
	lister Lister
}

func (c Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, v := range diskMetrics {
		ch <- v
	}
}

func (c Collector) Collect(ch chan<- prometheus.Metric) {
	disks, err := c.lister.List()
	if err != nil {
		klog.Errorf("Failed to get list of VirtualDisks: %v", err)
		return
	}
	if len(disks) == 0 {
		return
	}
	s := newScraper(ch)
	s.Report(disks)
}

func newScraper(ch chan<- prometheus.Metric) *scraper {
	return &scraper{ch: ch}
}

type scraper struct {
	ch chan<- prometheus.Metric
}

func (s *scraper) Report(disks []virtv2.VirtualDisk) {
	for _, d := range disks {
		s.updateDiskStatusPhaseMetrics(d)
	}
}

func (s *scraper) updateDiskStatusPhaseMetrics(disk virtv2.VirtualDisk) {
	phase := disk.Status.Phase
	if phase == "" {
		phase = virtv2.DiskPending
	}
	phases := []struct {
		value bool
		name  string
	}{
		{phase == virtv2.DiskPending, string(virtv2.DiskPending)},
		{phase == virtv2.DiskWaitForUserUpload, string(virtv2.DiskWaitForUserUpload)},
		{phase == virtv2.DiskProvisioning, string(virtv2.DiskProvisioning)},
		{phase == virtv2.DiskReady, string(virtv2.DiskReady)},
		{phase == virtv2.DiskFailed, string(virtv2.DiskFailed)},
		{phase == virtv2.DiskPVCLost, string(virtv2.DiskPVCLost)},
		{phase == virtv2.DiskUnknown, string(virtv2.DiskUnknown)},
	}
	desc := diskMetrics[MetricDiskStatusPhase]
	for _, p := range phases {
		metric, err := prometheus.NewConstMetric(
			desc,
			prometheus.GaugeValue,
			util.BoolFloat64(p.value),
			disk.GetName(), disk.GetNamespace(), string(disk.GetUID()), p.name,
		)
		if err != nil {
			klog.Warningf("Error creating the new const metric for %s: %s", desc, err)
			return
		}
		s.ch <- metric
	}
}
