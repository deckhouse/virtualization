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

package util

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// IPerfReport is the JSON report of an iperf3 client run ("iperf3 --json").
type IPerfReport struct {
	Start     IPerfReportStart      `json:"start"`
	Intervals []IPerfReportInterval `json:"intervals"`
	End       IPerfReportEnd        `json:"end"`
	Error     string                `json:"error,omitempty"`
}

type IPerfReportStart struct {
	Timestamp IPerfReportTimestamp `json:"timestamp"`
}

type IPerfReportTimestamp struct {
	Time     string `json:"time"`
	Timesecs int    `json:"timesecs"`
}

type IPerfReportEnd struct {
	SumSent     IPerfReportSummary `json:"sum_sent"`
	SumReceived IPerfReportSummary `json:"sum_received"`
}

type IPerfReportInterval struct {
	Sum IPerfReportSummary `json:"sum"`
}

type IPerfReportSummary struct {
	Bytes         int64   `json:"bytes"`
	BitsPerSecond float64 `json:"bits_per_second"`
	Start         float64 `json:"start,omitempty"`
	End           float64 `json:"end,omitempty"`
}

// StartTime is the wall-clock time at which the iperf test started, taken from
// the report header timestamp (RFC1123).
func (r *IPerfReport) GetStartTime() (time.Time, error) {
	return time.Parse(time.RFC1123, r.Start.Timestamp.Time)
}

// EndTime is the wall-clock time at which the iperf test stopped. iperf reports
// the run duration as seconds elapsed since the start timestamp
// (End.SumSent.End), so the absolute stop time is startTimesecs + duration.
func (r *IPerfReport) GetEndTime() time.Time {
	endSec := int64(r.Start.Timestamp.Timesecs) + int64(r.End.SumSent.End)
	frac := r.End.SumSent.End - float64(int64(r.End.SumSent.End))
	return time.Unix(endSec, int64(frac*1e9)).UTC()
}

// IntervalTime is the wall-clock time of an interval boundary, given as seconds
// since the start of the test.
func (r *IPerfReport) GetIntervalTime(offset float64) time.Time {
	base := time.Unix(int64(r.Start.Timestamp.Timesecs), 0).UTC()
	return base.Add(time.Duration(offset * float64(time.Second)))
}

// SilentIntervals returns the indexes of the intervals in which no bytes were
// transferred.
func (r *IPerfReport) FindSilentIntervals() []int {
	var silent []int
	for i, interval := range r.Intervals {
		if interval.Sum.Bytes == 0 {
			silent = append(silent, i)
		}
	}
	return silent
}

// IsInterruptedByClient reports whether the report error, if any, reflects the
// client's own SIGINT-triggered shutdown rather than a transport failure. The
// exact wording is not matched: the guest image carries whatever iperf3
// release Buildroot ships, so only the stable "interrupt" keyword is checked.
func (r *IPerfReport) IsInterruptedByClient() bool {
	if r.Error == "" {
		return true
	}
	return strings.Contains(strings.ToLower(r.Error), "interrupt")
}

func ParseIPerfReport(raw string) (*IPerfReport, error) {
	var report IPerfReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		return nil, fmt.Errorf("parse iperf3 json: %w", err)
	}
	return &report, nil
}
