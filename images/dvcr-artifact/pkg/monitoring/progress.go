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

package monitoring

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"k8s.io/klog/v2"
	"kubevirt.io/containerized-data-importer/pkg/common"
	"kubevirt.io/containerized-data-importer/pkg/util"
)

const (
	registryProgressName = "registry_progress"
	registryProgressHelp = "The registry import progress in percentage"
	registryAvgSpeedName = "registry_average_speed"
	registryAvgSpeedHelp = "The average registry import speed in bytes/sec"
	registryCurSpeedName = "registry_current_speed"
	registryCurSpeedHelp = "The current registry import speed in bytes/sec"
)

type ProgressMeter struct {
	*ProgressReader

	total                uint64
	avgSpeed             ProgressMetric
	curSpeed             ProgressMetric
	startedAt            time.Time
	stoppedAt            time.Time
	prevTransmittedBytes float64

	emitInterval time.Duration
	stop         chan struct{}

	cancel context.CancelFunc
}

// NewProgressMeter returns reader that will track bytes count into prometheus metric.
func NewProgressMeter(rdr io.ReadCloser, total uint64) *ProgressMeter {
	ownerUID, err := util.ParseEnvVar(common.OwnerUID, false)
	if err != nil {
		klog.Errorf("Failed to set owner UID for progress meter")
	}

	registryProgress := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: registryProgressName,
			Help: registryProgressHelp,
		},
		[]string{"ownerUID"},
	)

	err = prometheus.Register(registryProgress)
	if err != nil {
		var alreadyRegisteredErr prometheus.AlreadyRegisteredError

		if errors.As(err, &alreadyRegisteredErr) {
			// A counter for that metric has been registered before: use the old counter from now on.
			registryProgress = alreadyRegisteredErr.ExistingCollector.(*prometheus.GaugeVec)
		} else {
			klog.Errorf("Unable to create prometheus progress counter")
		}
	}

	registryAvgSpeed := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: registryAvgSpeedName,
			Help: registryAvgSpeedHelp,
		},
		[]string{"ownerUID"},
	)
	err = prometheus.Register(registryAvgSpeed)
	if err != nil {
		var alreadyRegisteredErr prometheus.AlreadyRegisteredError

		if errors.As(err, &alreadyRegisteredErr) {
			// A counter for that metric has been registered before: use the old gauge from now on.
			registryAvgSpeed = alreadyRegisteredErr.ExistingCollector.(*prometheus.GaugeVec)
		} else {
			klog.Errorf("Unable to create prometheus average progress speed gauge")
		}
	}

	registryCurSpeed := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: registryCurSpeedName,
			Help: registryCurSpeedHelp,
		},
		[]string{"ownerUID"},
	)
	err = prometheus.Register(registryCurSpeed)
	if err != nil {
		var alreadyRegisteredErr prometheus.AlreadyRegisteredError

		if errors.As(err, &alreadyRegisteredErr) {
			// A counter for that metric has been registered before: use the old gauge from now on.
			registryCurSpeed = alreadyRegisteredErr.ExistingCollector.(*prometheus.GaugeVec)
		} else {
			klog.Errorf("Unable to create prometheus current progress speed gauge")
		}
	}

	importProgress := NewProgress(registryProgress, ownerUID)

	ctx, cancel := context.WithCancel(context.Background())

	return &ProgressMeter{
		ProgressReader: NewProgressReader(ctx, rdr, importProgress, total),
		total:          total,
		avgSpeed:       NewProgress(registryAvgSpeed, ownerUID),
		curSpeed:       NewProgress(registryCurSpeed, ownerUID),
		emitInterval:   time.Second,
		stop:           make(chan struct{}),
		cancel:         cancel,
	}
}

func (p *ProgressMeter) Start() {
	p.ProgressReader.StartTimedUpdate()
	p.startedAt = time.Now()

	go func() {
		ticker := time.NewTicker(p.emitInterval)
		defer func() {
			p.cancel()
			ticker.Stop()
		}()

		for {
			select {
			case <-p.stop:
				return
			case <-ticker.C:
				if !p.updateSpeed() {
					return
				}
			}
		}
	}()
}

func (p *ProgressMeter) Stop() {
	close(p.stop)
	p.stoppedAt = time.Now()
}

func (p *ProgressMeter) GetAvgSpeed() uint64 {
	select {
	case <-p.stop:
		passedTime := float64(p.stoppedAt.Sub(p.startedAt).Nanoseconds()) / 1e9

		return uint64(float64(p.Current) / passedTime)
	default:
		passedTime := float64(time.Since(p.startedAt).Nanoseconds()) / 1e9

		return uint64(float64(p.Current) / passedTime)
	}
}

func (p *ProgressMeter) updateSpeed() bool {
	if p.total <= 0 {
		return false
	}

	finished := p.Done

	transmittedBytes := float64(p.total)
	if !finished && p.Current < p.total {
		transmittedBytes = float64(p.Current)
	}

	p.updateAvgSpeed(transmittedBytes)

	p.updateCurSpeed(transmittedBytes)

	return !finished
}

func (p *ProgressMeter) updateAvgSpeed(transmittedBytes float64) {
	passedTime := float64(time.Since(p.startedAt).Nanoseconds()) / 1e9
	avgSpeed := transmittedBytes / passedTime
	p.avgSpeed.Set(avgSpeed)
	klog.V(1).Infoln(fmt.Sprintf("Avg speed: %.2f b/s", avgSpeed))
}

func (p *ProgressMeter) updateCurSpeed(transmittedBytes float64) {
	diffBytes := transmittedBytes - p.prevTransmittedBytes
	p.prevTransmittedBytes = transmittedBytes
	curSpeed := diffBytes / p.emitInterval.Seconds()
	p.curSpeed.Set(curSpeed)
	klog.V(1).Infoln(fmt.Sprintf("Cur speed: %.2f b/s", curSpeed))
}

type ProgressMetric interface {
	Add(value float64)
	Set(value float64)
	Get() (float64, error)
	Delete()
}

func NewProgress(importProgress *prometheus.GaugeVec, labelValues ...string) *ImportProgress {
	return &ImportProgress{
		importProgress: importProgress,
		labelValues:    labelValues,
	}
}

type ImportProgress struct {
	importProgress *prometheus.GaugeVec
	labelValues    []string
}

// Add adds value to the importProgress metric
func (ip *ImportProgress) Add(value float64) {
	ip.importProgress.WithLabelValues(ip.labelValues...).Add(value)
}

func (ip *ImportProgress) Set(value float64) {
	ip.importProgress.WithLabelValues(ip.labelValues...).Set(value)
}

// Get returns the importProgress value
func (ip *ImportProgress) Get() (float64, error) {
	dtoMetric := &dto.Metric{}
	if err := ip.importProgress.WithLabelValues(ip.labelValues...).Write(dtoMetric); err != nil {
		return 0, err
	}
	return dtoMetric.Gauge.GetValue(), nil
}

// Delete removes the importProgress metric with the passed label
func (ip *ImportProgress) Delete() {
	ip.importProgress.DeleteLabelValues(ip.labelValues...)
}
