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
	registryAvgSpeedName = "registry_average_speed"
	registryAvgSpeedHelp = "The average registry import speed in bytes/sec"
	registryCurSpeedName = "registry_current_speed"
	registryCurSpeedHelp = "The current registry import speed in bytes/sec"
)

type ProgressMeter struct {
	*prometheusutil.ProgressReader

	total                uint64
	ownerUID             string
	avgSpeed             *prometheus.GaugeVec
	curSpeed             *prometheus.GaugeVec
	startedAt            time.Time
	stoppedAt            time.Time
	prevTransmittedBytes float64

	finalAvgSpeed float64

	emitInterval time.Duration
	stop         chan struct{}
}

// NewProgressMeter returns reader that will track bytes count into prometheus metric.
func NewProgressMeter(rdr io.ReadCloser, total uint64) *ProgressMeter {
	ownerUID, err := util.ParseEnvVar(common.OwnerUID, false)
	if err != nil {
		klog.Errorf("Failed to set owner UID for progress meter")
	}

	registryProgress := prometheus.NewCounterVec(
		prometheus.CounterOpts{
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
			registryProgress = alreadyRegisteredErr.ExistingCollector.(*prometheus.CounterVec)
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

	return &ProgressMeter{
		ProgressReader: prometheusutil.NewProgressReader(rdr, total, registryProgress, ownerUID),
		total:          total,
		ownerUID:       ownerUID,
		avgSpeed:       registryAvgSpeed,
		curSpeed:       registryCurSpeed,
		emitInterval:   time.Second,
		stop:           make(chan struct{}),
	}
}

func (p *ProgressMeter) Start() {
	p.ProgressReader.StartTimedUpdate()
	p.startedAt = time.Now()

	go func() {
		ticker := time.NewTicker(p.emitInterval)
		defer ticker.Stop()

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

	avgSpeedErr := p.updateAvgSpeed(transmittedBytes)
	if avgSpeedErr != nil {
		klog.Errorf("updateProgress: failed to read avg speed metric; %v", avgSpeedErr)
	}

	curSpeedErr := p.updateCurSpeed(transmittedBytes)
	if curSpeedErr != nil {
		klog.Errorf("updateProgress: failed to read cur speed metric; %v", curSpeedErr)
	}

	if avgSpeedErr != nil || curSpeedErr != nil {
		return true // true ==> to try again // todo - how to avoid endless loop in case it's a constant error?
	}

	return !finished
}

func (p *ProgressMeter) updateAvgSpeed(transmittedBytes float64) error {
	passedTime := float64(time.Since(p.startedAt).Nanoseconds()) / 1e9

	avgSpeed := transmittedBytes / passedTime

	err := p.avgSpeed.WithLabelValues(p.ownerUID).Write(&dto.Metric{})
	if err != nil {
		return err
	}
	p.avgSpeed.WithLabelValues(p.ownerUID).Set(avgSpeed)
	klog.V(1).Infoln(fmt.Sprintf("Avg speed: %.2f b/s", avgSpeed))

	return nil
}

func (p *ProgressMeter) updateCurSpeed(transmittedBytes float64) error {
	diffBytes := transmittedBytes - p.prevTransmittedBytes
	p.prevTransmittedBytes = transmittedBytes

	curSpeed := diffBytes / p.emitInterval.Seconds()

	err := p.curSpeed.WithLabelValues(p.ownerUID).Write(&dto.Metric{})
	if err != nil {
		return err
	}
	p.curSpeed.WithLabelValues(p.ownerUID).Set(curSpeed)
	klog.V(1).Infoln(fmt.Sprintf("Cur speed: %.2f b/s", curSpeed))

	return nil
}
