package monitoring

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"k8s.io/klog/v2"
	"kubevirt.io/containerized-data-importer/pkg/common"
	"kubevirt.io/containerized-data-importer/pkg/util"
	prometheusutil "kubevirt.io/containerized-data-importer/pkg/util/prometheus"
)

const (
	registryProgressName = "registry_progress"
	registryProgressHelp = "The registry import progress in percentage"
	registrySpeedName    = "registry_speed"
	registrySpeedHelp    = "The registry import speed in bytes/sec"
)

var (
	registryProgress = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: registryProgressName,
			Help: registryProgressHelp,
		},
		[]string{"ownerUID"},
	)
	registrySpeed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: registrySpeedName,
			Help: registrySpeedHelp,
		},
		[]string{"ownerUID"},
	)
	ownerUID string
)

type ProgressMeterReader struct {
	*prometheusutil.ProgressReader
	speed    *prometheus.GaugeVec
	total    uint64
	ownerUID string
}

func init() {
	if err := prometheus.Register(registryProgress); err != nil {
		var alreadyRegisteredErr prometheus.AlreadyRegisteredError

		if errors.As(err, &alreadyRegisteredErr) {
			// A counter for that metric has been registered before.
			// Use the old counter from now on.
			registryProgress = alreadyRegisteredErr.ExistingCollector.(*prometheus.CounterVec)
		} else {
			klog.Errorf("Unable to create prometheus progress counter")
		}
	}
	if err := prometheus.Register(registrySpeed); err != nil {
		var alreadyRegisteredErr prometheus.AlreadyRegisteredError

		if errors.As(err, &alreadyRegisteredErr) {
			// A counter for that metric has been registered before.
			// Use the old gauge from now on.
			registrySpeed = alreadyRegisteredErr.ExistingCollector.(*prometheus.GaugeVec)
		} else {
			klog.Errorf("Unable to create prometheus progress speed gauge")
		}
	}
	ownerUID, _ = util.ParseEnvVar(common.OwnerUID, false)
}

// NewProgressMeterReader returns reader that will track bytes count into prometheus metric.
func NewProgressMeterReader(rdr io.ReadCloser, total uint64) *ProgressMeterReader {
	return &ProgressMeterReader{
		ProgressReader: prometheusutil.NewProgressReader(rdr, total, registryProgress, ownerUID),
		speed:          registrySpeed,
		total:          total,
		ownerUID:       ownerUID,
	}
}

func (p *ProgressMeterReader) StartTimedUpdate() {
	// Start the progress update thread.
	p.ProgressReader.StartTimedUpdate()
	go p.timedUpdateSpeed()
}

func (p *ProgressMeterReader) timedUpdateSpeed() {
	cont := true
	start := time.Now()
	for cont {
		// Update every second.
		time.Sleep(time.Second)
		cont = p.updateSpeed(start)
	}
}

func (p *ProgressMeterReader) updateSpeed(start time.Time) bool {
	if p.total > 0 {
		finished := p.Done
		passedTime := float64(time.Since(start).Nanoseconds()) / 1e9
		transmittedBytes := float64(p.total)
		if !finished && p.Current < p.total {
			transmittedBytes = float64(p.Current)
		}
		currentSpeed := transmittedBytes / passedTime
		metric := &dto.Metric{}
		if err := p.speed.WithLabelValues(p.ownerUID).Write(metric); err != nil {
			klog.Errorf("updateProgress: failed to read metric; %v", err)
			return true // true ==> to try again // todo - how to avoid endless loop in case it's a constant error?
		}
		p.speed.WithLabelValues(p.ownerUID).Set(currentSpeed)
		klog.V(1).Infoln(fmt.Sprintf("%.2f b/s", currentSpeed))
		return !finished
	}
	return false
}
