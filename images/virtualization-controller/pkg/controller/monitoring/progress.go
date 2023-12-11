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

	"github.com/dustin/go-humanize"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/net"
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

// extractProgress parses final report and extracts metrics:
// registry_progress{ownerUID="b856691e-1038-11e9-a5ab-525500d15501"} 47.68095477934807
// registry_current_speed{ownerUID="b856691e-1038-11e9-a5ab-525500d15501"} 2.12e+06
// registry_average_speed{ownerUID="b856691e-1038-11e9-a5ab-525500d15501"} 2.3832862149406234e+06
func extractProgress(report, ownerUID string) (*ImportProgress, error) {
	if report == "" {
		return nil, nil
	}

	// Note: invalid float format will be checked later using ParseFloat.
	progressRe := regexp.MustCompile(`registry_progress\{ownerUID\="` + ownerUID + `"\} ([0-9e\+\-\.]+)`)
	avgSpeedRe := regexp.MustCompile(`registry_average_speed\{ownerUID\="` + ownerUID + `"\} ([0-9e\+\-\.]+)`)
	curSpeedRe := regexp.MustCompile(`registry_current_speed\{ownerUID\="` + ownerUID + `"\} ([0-9e\+\-\.]+)`)

	res := &ImportProgress{}

	match := progressRe.FindStringSubmatch(report)
	if match != nil {
		raw := match[1]
		val, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("parse registry_progress metric: %w", err)
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
	return humanize.Bytes(p.curSpeed) + "/s"
}

// CurSpeedRaw is a current in bytes per second.
func (p *ImportProgress) CurSpeedRaw() uint64 {
	return p.curSpeed
}

// AvgSpeed is an average speed in human-readable format with SI size.
func (p *ImportProgress) AvgSpeed() string {
	return humanize.Bytes(p.avgSpeed) + "/s"
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
