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
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/net"

	"github.com/deckhouse/virtualization-controller/pkg/common/humanize_bytes"
)

var httpClient *http.Client

type ImportProgress struct {
	progress float64
	avgSpeed uint64
	curSpeed uint64
}

func GetImportProgressFromPod(ownerUID string, pod *corev1.Pod) (*ImportProgress, error) {
	httpClient = BuildHTTPClient(httpClient)
	url, err := GetMetricsURL(pod)
	if err != nil {
		return nil, err
	}
	if url == "" {
		return nil, nil
	}

	progressReport, err := GetProgressReportFromURL(url, httpClient)
	if err != nil {
		return nil, err
	}
	return extractProgress(progressReport, ownerUID)
}

// extractProgress parses the final report and extracts metrics. Two metric
// families are recognized:
//
//   - registry_progress / registry_current_speed / registry_average_speed are
//     emitted by dvcr-importer / dvcr-uploader pods (the "first half" import
//     into DVCR for HTTP / Registry / Upload data sources).
//   - kubevirt_cdi_import_progress_total is emitted by the cdi-importer pod
//     (the "second half" import from DVCR into the target PVC; for ObjectRef
//     CVI / VI it is also the only import pod).
//
// Example lines:
//
//	registry_progress{ownerUID="b856691e-1038-11e9-a5ab-525500d15501"} 47.6809
//	registry_current_speed{ownerUID="b856691e-1038-11e9-a5ab-525500d15501"} 2.12e+06
//	registry_average_speed{ownerUID="b856691e-1038-11e9-a5ab-525500d15501"} 2.38e+06
//	kubevirt_cdi_import_progress_total{ownerUID="b856691e-1038-11e9-a5ab-525500d15501"} 73.42
func extractProgress(report, ownerUID string) (*ImportProgress, error) {
	if report == "" {
		return nil, nil
	}

	// Note: invalid float format will be checked later using ParseFloat.
	// Match either the dvcr-importer's registry_progress or the cdi-importer's
	// kubevirt_cdi_import_progress_total metric. Both are reported in the same
	// 0..100 scale, so either value is a valid pod-local progress percentage.
	progressRe := regexp.MustCompile(`(?:registry_progress|kubevirt_cdi_import_progress_total)\{ownerUID\="` + ownerUID + `"\} ([0-9e\+\-\.]+)`)
	avgSpeedRe := regexp.MustCompile(`registry_average_speed\{ownerUID\="` + ownerUID + `"\} ([0-9e\+\-\.]+)`)
	curSpeedRe := regexp.MustCompile(`registry_current_speed\{ownerUID\="` + ownerUID + `"\} ([0-9e\+\-\.]+)`)

	res := &ImportProgress{}

	match := progressRe.FindStringSubmatch(report)
	if match != nil {
		raw := match[1]
		val, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("parse import progress metric: %w", err)
		}
		res.progress = val
	}

	match = avgSpeedRe.FindStringSubmatch(report)
	if match != nil {
		raw := match[1]
		val, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("parse registry_average_speed metric: %w", err)
		}
		res.avgSpeed = uint64(val)
	}

	match = curSpeedRe.FindStringSubmatch(report)
	if match != nil {
		raw := match[1]
		val, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("parse registry_current_speed metric: %w", err)
		}
		res.curSpeed = uint64(val)
	}

	return res, nil
}

func (p *ImportProgress) Progress() string {
	return fmt.Sprintf("%.1f%%", p.progress)
}

func (p *ImportProgress) ProgressRaw() float64 {
	return p.progress
}

// CurSpeed is a current speed in human-readable format with SI size.
func (p *ImportProgress) CurSpeed() string {
	return humanize_bytes.HumanizeIBytes(p.curSpeed) + "/s"
}

// CurSpeedRaw is a current in bytes per second.
func (p *ImportProgress) CurSpeedRaw() uint64 {
	return p.curSpeed
}

// AvgSpeed is an average speed in human-readable format with SI size.
func (p *ImportProgress) AvgSpeed() string {
	return humanize_bytes.HumanizeIBytes(p.avgSpeed) + "/s"
}

// AvgSpeedRaw is an average speed in bytes per second.
func (p *ImportProgress) AvgSpeedRaw() uint64 {
	return p.avgSpeed
}

// BuildHTTPClient generates an http client that accepts any certificate, since we are using
// it to get prometheus data it doesn't matter if someone can intercept the data. Once we have
// a mechanism to properly sign the server, we can update this method to get a proper client.
func BuildHTTPClient(httpClient *http.Client) *http.Client {
	if httpClient == nil {
		defaultTransport := http.DefaultTransport.(*http.Transport)
		// Create new Transport that ignores self-signed SSL
		tr := &http.Transport{
			Proxy:                 defaultTransport.Proxy,
			DialContext:           defaultTransport.DialContext,
			MaxIdleConns:          defaultTransport.MaxIdleConns,
			IdleConnTimeout:       defaultTransport.IdleConnTimeout,
			ExpectContinueTimeout: defaultTransport.ExpectContinueTimeout,
			TLSHandshakeTimeout:   defaultTransport.TLSHandshakeTimeout,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		}
		httpClient = &http.Client{
			Transport: tr,
			Timeout:   time.Second,
		}
	}
	return httpClient
}

// GetPodMetricsPort returns, if exists, the metrics port from the passed pod
func GetPodMetricsPort(pod *corev1.Pod) (int, error) {
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			if port.Name == "metrics" {
				return int(port.ContainerPort), nil
			}
		}
	}
	return 0, fmt.Errorf("metrics port not found in pod %s", pod.Name)
}

// GetProgressReportFromURL fetches the progress report from the passed URL according to a specific regular expression
func GetProgressReportFromURL(url string, httpClient *http.Client) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		if net.IsConnectionRefused(err) {
			return "", nil
		}
		if net.IsTimeout(err) {
			return "", nil
		}
		if strings.Contains(err.Error(), "no route to host") {
			return "", nil
		}

		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// GetMetricsURL builds the metrics URL according to the specified pod
func GetMetricsURL(pod *corev1.Pod) (string, error) {
	if pod == nil {
		return "", nil
	}
	port, err := GetPodMetricsPort(pod)
	if err != nil || pod.Status.PodIP == "" {
		return "", err
	}
	url := fmt.Sprintf("https://%s:%d/metrics", pod.Status.PodIP, port)
	return url, nil
}
