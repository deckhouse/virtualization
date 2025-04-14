/*
Copyright 2025 Flant JSC

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
	"fmt"
	"io"
	"time"

	"k8s.io/klog/v2"

	"kubevirt.io/containerized-data-importer/pkg/util"
)

// ProgressReader is a counting reader that reports progress to prometheus.
type ProgressReader struct {
	util.CountingReader
	metric ProgressMetric
	total  uint64
	final  bool
	ctx    context.Context
}

// NewProgressReader creates a new instance of a prometheus updating progress reader.
func NewProgressReader(r io.ReadCloser, metric ProgressMetric, total uint64) *ProgressReader {
	promReader := &ProgressReader{
		CountingReader: util.CountingReader{
			Reader:  r,
			Current: 0,
		},
		metric: metric,
		total:  total,
		final:  true,
	}

	return promReader
}

// StartTimedUpdate starts the update timer to automatically update every second.
func (r *ProgressReader) StartTimedUpdate(ctx context.Context) {
	r.ctx = ctx
	// Start the progress update thread.
	go r.timedUpdateProgress()
}

func (r *ProgressReader) timedUpdateProgress() {
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-time.After(time.Second):
			cont := r.updateProgress()
			if !cont {
				return
			}
		}
	}
}

func (r *ProgressReader) updateProgress() bool {
	if r.total > 0 {
		finished := r.final && r.Done
		currentProgress := 100.0
		if !finished && r.Current < r.total {
			currentProgress = float64(r.Current) / float64(r.total) * 100.0
		}
		progress, err := r.metric.Get()
		if err != nil {
			klog.Errorf("updateProgress: failed to read metric; %v", err)
			return true // true ==> to try again // todo - how to avoid endless loop in case it's a constant error?
		}
		if currentProgress > progress {
			r.metric.Add(currentProgress - progress)
		}
		klog.V(1).Infoln(fmt.Sprintf("%.2f", currentProgress))
		return !finished
	}
	return false
}
