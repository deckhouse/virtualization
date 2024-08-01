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

package vmbda

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"

	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics"
	"github.com/deckhouse/virtualization-controller/pkg/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	MetricVMBDAStatusPhase = "virtualmachineblockdeviceattachment_status_phase"
)

var vmbdaMetrics = map[string]*prometheus.Desc{
	MetricVMBDAStatusPhase: prometheus.NewDesc(prometheus.BuildFQName(metrics.MetricNamespace, "", MetricVMBDAStatusPhase),
		"The virtualmachineblockdeviceattachment current phase.",
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
	List(ctx context.Context) ([]virtv2.VirtualMachineBlockDeviceAttachment, error)
}

type Collector struct {
	lister Lister
}

func (c Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, v := range vmbdaMetrics {
		ch <- v
	}
}

func (c Collector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vmbdas, err := c.lister.List(ctx)
	if err != nil {
		klog.Errorf("Failed to get list of VirtualMachineBlockDeviceAttachment: %v", err)
		return
	}
	if len(vmbdas) == 0 {
		return
	}
	s := newScraper(ch)
	s.Report(vmbdas)
}

func newScraper(ch chan<- prometheus.Metric) *scraper {
	return &scraper{ch: ch}
}

type scraper struct {
	ch chan<- prometheus.Metric
}

func (s *scraper) Report(vmbdas []virtv2.VirtualMachineBlockDeviceAttachment) {
	for _, vmbda := range vmbdas {
		s.updateVMBDAStatusPhaseMetrics(vmbda)
	}
}

func (s *scraper) updateVMBDAStatusPhaseMetrics(vmbda virtv2.VirtualMachineBlockDeviceAttachment) {
	phase := vmbda.Status.Phase
	if phase == "" {
		phase = virtv2.BlockDeviceAttachmentPhasePending
	}
	phases := []struct {
		value bool
		name  string
	}{
		{phase == virtv2.BlockDeviceAttachmentPhasePending, string(virtv2.BlockDeviceAttachmentPhasePending)},
		{phase == virtv2.BlockDeviceAttachmentPhaseInProgress, string(virtv2.BlockDeviceAttachmentPhaseInProgress)},
		{phase == virtv2.BlockDeviceAttachmentPhaseAttached, string(virtv2.BlockDeviceAttachmentPhaseAttached)},
		{phase == virtv2.BlockDeviceAttachmentPhaseFailed, string(virtv2.BlockDeviceAttachmentPhaseFailed)},
		{phase == virtv2.BlockDeviceAttachmentPhaseTerminating, string(virtv2.BlockDeviceAttachmentPhaseTerminating)},
	}
	desc := vmbdaMetrics[MetricVMBDAStatusPhase]
	for _, p := range phases {
		metric, err := prometheus.NewConstMetric(
			desc,
			prometheus.GaugeValue,
			util.BoolFloat64(p.value),
			vmbda.GetName(), vmbda.GetNamespace(), string(vmbda.GetUID()), p.name,
		)
		if err != nil {
			klog.Warningf("Error creating the new const metric for %s: %s", desc, err)
			return
		}
		s.ch <- metric
	}
}
