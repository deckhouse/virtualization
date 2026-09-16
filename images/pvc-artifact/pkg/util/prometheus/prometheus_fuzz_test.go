/*
Copyright 2026 Flant JSC

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

package prometheus

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	pvcimporter "github.com/deckhouse/virtualization/images/pvc-artifact/pkg/monitoring/metrics/pvc-importer"
)

const (
	fuzzMaxRequestPart = 4 << 10
	fuzzMaxBody        = 64 << 10
	fuzzMaxResponse    = 4 << 20
	fuzzDialTimeout    = 5 * time.Second

	// Short on purpose: an unparseable request otherwise waits out the server's
	// ten second ReadHeaderTimeout.
	fuzzReadTimeout = 200 * time.Millisecond
)

// fuzzAllowedFamilyPrefixes is a canary on what the port exposes: the port has
// no authentication, so a new family here must be a decision, not an accident.
var fuzzAllowedFamilyPrefixes = []string{
	"kubevirt_cdi_",
	"go_",
	"process_",
	"promhttp_",
}

// FuzzPrometheusEndpoint drives the metrics endpoint of the importer Pods.
// Requests go into the connection as raw bytes rather than through an
// http.Client, which would refuse the malformed request lines worth trying.
//
// The target sits on this pvc-artifact copy of the CDI helper because
// dvcr-artifact gets the same code from a dependency and cannot host one.
//
// Checked: one hostile request must not take the endpoint away from the
// monitoring, and the port must expose only the families it is expected to.
func FuzzPrometheusEndpoint(f *testing.F) {
	// net/http logs every failed TLS handshake, and the raw-bytes path below
	// fails one in every iteration.
	log.SetOutput(io.Discard)
	f.Cleanup(func() { log.SetOutput(os.Stderr) })

	// Register the counters the way pvc-importer does.
	if err := pvcimporter.SetupMetrics(); err != nil {
		f.Fatalf("failed to register the importer metrics: %v", err)
	}

	// A counter vector with no label values renders no lines at all.
	pvcimporter.Progress("fuzz-owner-uid").Add(1)

	listener, err := startPrometheusEndpoint(f.TempDir(), "127.0.0.1:0")
	if err != nil {
		f.Fatalf("failed to start the metrics endpoint: %v", err)
	}
	f.Cleanup(func() { _ = listener.Close() })

	addr := listener.Addr().String()

	// One client for the whole run: a fresh one per input would redo the TLS
	// handshake every time.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				// The endpoint's certificate is self signed by design.
				InsecureSkipVerify: true, //nolint:gosec // gosec:G402 the endpoint's certificate is self signed by design; this target checks the listener, not the trust chain
			},
		},
		Timeout: fuzzDialTimeout,
	}
	f.Cleanup(client.CloseIdleConnections)

	// Ordinary scrapes.
	f.Add("GET", "/metrics", "text/plain", "", "", []byte(nil))
	f.Add("GET", "/metrics", "application/openmetrics-text; version=1.0.0", "gzip", "10", []byte(nil))
	f.Add("GET", "/metrics", "application/vnd.google.protobuf; proto=io.prometheus.client.MetricFamily; encoding=delimited", "", "", []byte(nil))
	f.Add("HEAD", "/metrics", "text/plain", "", "", []byte(nil))
	// Any URL answers: there is no path routing.
	f.Add("GET", "/", "text/plain", "", "", []byte(nil))
	f.Add("GET", "/healthz", "text/plain", "", "", []byte(nil))
	f.Add("GET", "/../../etc/passwd", "text/plain", "", "", []byte(nil))
	f.Add("GET", "//metrics", "text/plain", "", "", []byte(nil))
	f.Add("GET", "/metrics?debug=1&"+strings.Repeat("a=1&", 512), "text/plain", "", "", []byte(nil))
	// Methods the handler does not expect.
	f.Add("POST", "/metrics", "text/plain", "", "", []byte("body"))
	f.Add("PUT", "/metrics", "text/plain", "", "", []byte("body"))
	f.Add("DELETE", "/metrics", "text/plain", "", "", []byte(nil))
	f.Add("TRACE", "/metrics", "text/plain", "", "", []byte(nil))
	f.Add("CONNECT", "127.0.0.1:1", "", "", "", []byte(nil))
	f.Add("OPTIONS", "*", "", "", "", []byte(nil))
	f.Add("get", "/metrics", "text/plain", "", "", []byte(nil))
	f.Add("", "/metrics", "text/plain", "", "", []byte(nil))
	f.Add(strings.Repeat("A", 1024), "/metrics", "", "", "", []byte(nil))
	f.Add("GET\tSP", "/metrics", "", "", "", []byte(nil))
	// Format negotiation, the one header the handler reads.
	f.Add("GET", "/metrics", strings.Repeat("text/plain,", 256)+"text/plain", "", "", []byte(nil))
	f.Add("GET", "/metrics", "*/*;q=", "", "", []byte(nil))
	f.Add("GET", "/metrics", "application/openmetrics-text; version=99999999999", "", "", []byte(nil))
	f.Add("GET", "/metrics", "\x00\x01\x02", "", "", []byte(nil))
	f.Add("GET", "/metrics", "\xff\xfe", "", "", []byte(nil))
	f.Add("GET", "/metrics", "text/plain; version=0.0.4; charset=\"", "", "", []byte(nil))
	// Content coding, including codings the handler cannot produce.
	f.Add("GET", "/metrics", "text/plain", "gzip, deflate, br, zstd, identity", "", []byte(nil))
	f.Add("GET", "/metrics", "text/plain", strings.Repeat("gzip,", 512), "", []byte(nil))
	f.Add("GET", "/metrics", "text/plain", "gzip;q=0", "", []byte(nil))
	f.Add("GET", "/metrics", "text/plain", "\x00", "", []byte(nil))
	// The scrape timeout header the Prometheus server sends.
	f.Add("GET", "/metrics", "text/plain", "", "0", []byte(nil))
	f.Add("GET", "/metrics", "text/plain", "", "0.000000001", []byte(nil))
	f.Add("GET", "/metrics", "text/plain", "", "-1", []byte(nil))
	f.Add("GET", "/metrics", "text/plain", "", "99999999999999999999", []byte(nil))
	f.Add("GET", "/metrics", "text/plain", "", "NaN", []byte(nil))
	f.Add("GET", "/metrics", "text/plain", "", "not-a-number", []byte(nil))
	f.Add("GET", "/metrics", "text/plain", "", "\x00", []byte(nil))
	// Bodies, including a declared length that does not match what follows.
	f.Add("POST", "/metrics", "text/plain", "", "", bytes.Repeat([]byte{0x00}, 4096))
	f.Add("POST", "/metrics", "text/plain", "", "", bytes.Repeat([]byte{0xff}, 4096))
	f.Add("POST", "/metrics", "text/plain", "", "", []byte("\r\n\r\nGET /metrics HTTP/1.1\r\n\r\n"))
	// Header and request-line shapes that are not valid HTTP at all.
	f.Add("GET", "/metrics\r\nX-Injected: 1", "text/plain", "", "", []byte(nil))
	f.Add("GET", "/metrics", "text/plain\r\nX-Injected: 1", "", "", []byte(nil))
	f.Add("GET", "/metrics", "text/plain", "", "1\r\nX-Injected: 1", []byte(nil))
	f.Add("GET", " ", "", "", "", []byte(nil))
	f.Add("GET", "", "", "", "", []byte(nil))
	f.Add("\x00", "\x00", "\x00", "\x00", "\x00", []byte{0x00})

	f.Fuzz(func(t *testing.T, method, target, accept, encoding, scrapeTimeout string, body []byte) {
		if len(method) > fuzzMaxRequestPart ||
			len(target) > fuzzMaxRequestPart ||
			len(accept) > fuzzMaxRequestPart ||
			len(encoding) > fuzzMaxRequestPart ||
			len(scrapeTimeout) > fuzzMaxRequestPart ||
			len(body) > fuzzMaxBody {
			t.Skip()
		}

		// Bytes that are not a TLS handshake.
		writeRawBytes(addr, body)

		// The prepared request, written to the TLS connection verbatim.
		response := writeRawRequest(t, addr, method, target, accept, encoding, scrapeTimeout, body)
		if len(response) > fuzzMaxResponse {
			t.Fatalf("a %d byte request produced a %d byte response, past the bound of %d", len(body), len(response), fuzzMaxResponse)
		}

		// The endpoint has to still answer the monitoring afterwards.
		requireEndpointStillServes(t, client, addr)
	})
}

// writeRawBytes writes bytes that are not a TLS handshake. Nothing is asserted
// here: the check is the caller's scrape.
func writeRawBytes(addr string, data []byte) {
	conn, err := net.DialTimeout("tcp", addr, fuzzDialTimeout)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetDeadline(time.Now().Add(fuzzReadTimeout))
	_, _ = conn.Write(data)
	_, _ = io.Copy(io.Discard, io.LimitReader(conn, 4096))
}

// writeRawRequest assembles an HTTP request out of the fuzzed pieces and writes
// it byte for byte. Refusing it is the normal outcome.
func writeRawRequest(t *testing.T, addr, method, target, accept, encoding, scrapeTimeout string, body []byte) []byte {
	t.Helper()

	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: fuzzDialTimeout},
		Config: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // gosec:G402 see the client above
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), fuzzDialTimeout)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetDeadline(time.Now().Add(fuzzReadTimeout))

	var request bytes.Buffer

	fmt.Fprintf(&request, "%s %s HTTP/1.1\r\n", method, target)
	fmt.Fprintf(&request, "Host: %s\r\n", addr)
	if accept != "" {
		fmt.Fprintf(&request, "Accept: %s\r\n", accept)
	}
	if encoding != "" {
		fmt.Fprintf(&request, "Accept-Encoding: %s\r\n", encoding)
	}
	if scrapeTimeout != "" {
		fmt.Fprintf(&request, "X-Prometheus-Scrape-Timeout-Seconds: %s\r\n", scrapeTimeout)
	}
	fmt.Fprintf(&request, "Content-Length: %d\r\n", len(body))
	request.WriteString("Connection: close\r\n\r\n")
	request.Write(body)

	if _, err := conn.Write(request.Bytes()); err != nil {
		return nil
	}

	response, _ := io.ReadAll(io.LimitReader(conn, fuzzMaxResponse+1))

	return response
}

// requireEndpointStillServes scrapes the endpoint the way the monitoring does.
// Family names are read out of the text exposition format directly, which
// avoids a dependency the module does not otherwise have.
func requireEndpointStillServes(t *testing.T, client *http.Client, addr string) {
	t.Helper()

	resp, err := client.Get("https://" + addr + "/metrics")
	if err != nil {
		t.Fatalf("the metrics endpoint stopped answering after the request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the metrics endpoint answered a plain scrape with %s", resp.Status)
	}

	payload, err := io.ReadAll(io.LimitReader(resp.Body, fuzzMaxResponse+1))
	if err != nil {
		t.Fatalf("failed to read the scrape after the request: %v", err)
	}

	if len(payload) > fuzzMaxResponse {
		t.Fatalf("the scrape is larger than the bound of %d bytes", fuzzMaxResponse)
	}

	families := metricFamilies(payload)
	if len(families) == 0 {
		t.Fatal("the scrape carries no metrics at all")
	}

	if !families["kubevirt_cdi_import_progress_total"] {
		t.Fatal("the scrape does not carry the import progress counter the module registers")
	}

	for name := range families {
		if !hasAllowedPrefix(name) {
			t.Fatalf(
				"the metrics port exposes the family %q, which is not in the expected set %v; the port has no authentication, so anything reaching it can read this",
				name, fuzzAllowedFamilyPrefixes,
			)
		}
	}
}

// metricFamilies returns the family names present in a text format exposition.
func metricFamilies(payload []byte) map[string]bool {
	families := make(map[string]bool)

	for line := range strings.SplitSeq(string(payload), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		name := line
		if i := strings.IndexAny(name, "{ "); i >= 0 {
			name = name[:i]
		}

		if name != "" {
			families[name] = true
		}
	}

	return families
}

func hasAllowedPrefix(name string) bool {
	for _, prefix := range fuzzAllowedFamilyPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}
