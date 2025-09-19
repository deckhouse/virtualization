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

package fuzz

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	"github.com/hashicorp/go-cleanhttp"
)

func ProcessRequests(tb *testing.T, data []byte, addr string, methods ...string) {
	tb.Helper()

	if len(methods) == 0 {
		tb.Fatalf("no methods specified")
	}
	for _, method := range methods {
		ProcessRequest(tb, data, addr, method)
	}
}

func ProcessRequest(tb testing.TB, data []byte, addr, method string) {
	tb.Helper()

	switch method {
	case
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodConnect,
		http.MethodOptions,
		http.MethodTrace:

		req := newFuzzRequest().Fuzz(tb, data, method, addr)
		defer req.Body.Close()

		resp := fuzzHTTPRequest(tb, req)
		if resp != nil {
			if resp.StatusCode > 500 {
				tb.Errorf("resp: %v", resp)
			}
			defer resp.Body.Close()
		}
	default:
		tb.Errorf("Unsupported HTTP method: %s", method)
	}
}

func fuzzHTTPRequest(tb testing.TB, fuzzReq *http.Request) *http.Response {
	tb.Helper()

	if fuzzReq == nil {
		tb.Skip("Skipping test because fuzzReq is nil")
	}
	client := cleanhttp.DefaultClient()
	client.Timeout = time.Second

	// From https://github.com/michiwend/gomusicbrainz/pull/4/files
	// Redirect limit is set to 30 to avoid loosing userAgent and other headers
	// 30 is good middle ground between not being too strict and not being too lenient
	const defaultRedirectLimit = 30

	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > defaultRedirectLimit {
			return fmt.Errorf("%d consecutive requests(redirects)", len(via))
		}
		if len(via) == 0 {
			// No redirects
			return nil
		}

		return nil
	}

	tb.Logf("fuzzing request, %s, %s", fuzzReq.Method, fuzzReq.URL)

	resp, err := client.Do(fuzzReq)
	if err != nil && !strings.Contains(err.Error(), "checkRedirect disabled for test") {
		tb.Logf("err: %s", err)
	}

	return resp
}

type fuzzRequest struct{}

func newFuzzRequest() *fuzzRequest {
	return &fuzzRequest{}
}

func (s *fuzzRequest) Fuzz(tb testing.TB, data []byte, method, addr string) *http.Request {
	tb.Helper()

	bodyReader := bytes.NewBuffer(data)

	req, err := http.NewRequest(method, addr, bodyReader)
	if err != nil {
		tb.Skipf("Skipping test: not enough data for fuzzing: %s", err.Error())
	}

	// Get the address of the local listener in order to attach it to an Origin header.
	// This will allow for the testing of requests that require CORS, without using a browser.
	hostURLRegexp := regexp.MustCompile("http[s]?://.+:[0-9]+")

	fuzzConsumer := fuzz.NewConsumer(data)
	var headersMap map[string]string
	err = fuzzConsumer.FuzzMap(&headersMap)
	if err != nil {
		tb.Skipf("Skipping test: not enough data for fuzzing: %s", err.Error())
	}

	for k, v := range headersMap {
		for range len(v) {
			req.Header.Add(k, v)
		}
	}

	req.Header.Set("Origin", hostURLRegexp.FindString(addr))
	req.Header.Set("Content-Length", strconv.Itoa(len(data)))
	req.Header.Set("Content-Type", "application/octet-stream")

	return req
}
