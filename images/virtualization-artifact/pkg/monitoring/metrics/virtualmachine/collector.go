package virtualmachine

import (
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"

	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
	"github.com/deckhouse/virtualization-controller/pkg/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	MetricVirtualMachineStatusPhase = "virtualmachine_status_phase"
)

var virtualMachineMetrics = map[string]*prometheus.Desc{
	MetricVirtualMachineStatusPhase: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVirtualMachineStatusPhase),
		"The virtualmachine current phase.",
		[]string{"name", "namespace", "uid", "node", "phase"},
		nil),
}

func SetupCollector(vmLister Lister, registerer prometheus.Registerer) *Collector {
	c := &Collector{
		lister: vmLister,
	}

	registerer.MustRegister(c)
	return c
}

type Lister interface {
	List() ([]virtv2.VirtualMachine, error)
}

type Collector struct {
	lister Lister
}

func (c Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, v := range virtualMachineMetrics {
		ch <- v
	}
}

func (c Collector) Collect(ch chan<- prometheus.Metric) {
	vms, err := c.lister.List()
	if len(vms) == 0 || err != nil {
		return
	}
	scraper := newScraper(ch)
	scraper.Report(vms)
}

func newScraper(ch chan<- prometheus.Metric) *scraper {
	return &scraper{ch: ch}
}

type scraper struct {
	ch chan<- prometheus.Metric
}

func (s *scraper) Report(vms []virtv2.VirtualMachine) {
	for _, vm := range vms {
		s.updateVMStatusPhaseMetrics(vm)
	}
}

func (s *scraper) updateVMStatusPhaseMetrics(vm virtv2.VirtualMachine) {
	phase := vm.Status.Phase
	if phase == "" {
		phase = virtv2.MachinePending
	}
	phases := []struct {
		v bool
		n string
	}{
		{phase == virtv2.MachineScheduling, string(virtv2.MachineScheduling)},
		{phase == virtv2.MachinePending, string(virtv2.MachinePending)},
		{phase == virtv2.MachineRunning, string(virtv2.MachineRunning)},
		{phase == virtv2.MachineFailed, string(virtv2.MachineFailed)},
		{phase == virtv2.MachineTerminating, string(virtv2.MachineTerminating)},
		{phase == virtv2.MachineStopped, string(virtv2.MachineStopped)},
		{phase == virtv2.MachineStopping, string(virtv2.MachineStopping)},
		{phase == virtv2.MachineStarting, string(virtv2.MachineStarting)},
		{phase == virtv2.MachineMigrating, string(virtv2.MachineMigrating)},
		{phase == virtv2.MachinePause, string(virtv2.MachinePause)},
	}
	desc := virtualMachineMetrics[MetricVirtualMachineStatusPhase]
	for _, p := range phases {
		mv, err := prometheus.NewConstMetric(
			desc,
			prometheus.GaugeValue,
			util.BoolFloat64(p.v),
			vm.GetName(), vm.GetNamespace(), string(vm.GetUID()), vm.Status.NodeName, p.n,
		)
		if err != nil {
			klog.Warningf("Error creating the new const metric for %s: %s", desc, err)
			return
		}
		s.ch <- mv
	}
}
