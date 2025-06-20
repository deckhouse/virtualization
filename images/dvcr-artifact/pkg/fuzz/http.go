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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	"github.com/hashicorp/go-cleanhttp"
)

func ProcessRequests(t *testing.T, data []byte, addr string, methods ...string) {
	t.Helper()

	if len(methods) == 0 {
		t.Fatalf("no methods specified")
	}
	for _, method := range methods {
		ProcessRequest(t, data, addr, method)
	}
}

func ProcessRequest(t *testing.T, data []byte, addr, method string) {
	t.Helper()

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

		req := newFuzzRequest().Fuzz(t, data, method, addr)
		defer req.Body.Close()

		resp := fuzzHTTPRequest(t, req)
		if resp != nil {
			if resp.StatusCode > 500 {
				t.Errorf("resp: %v", resp)
			}
			defer resp.Body.Close()
		}
	default:
		t.Errorf("Unsupported HTTP method: %s", method)
	}
}

func fuzzHTTPRequest(t *testing.T, fuzzReq *http.Request) *http.Response {
	t.Helper()

	if fuzzReq == nil {
		t.Skip("Skipping test because fuzzReq is nil")
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

	t.Logf("fuzzing request, %s, %s", fuzzReq.Method, fuzzReq.URL)

	resp, err := client.Do(fuzzReq)
	if err != nil && !strings.Contains(err.Error(), "checkRedirect disabled for test") {
		t.Logf("err: %s", err)
	}

	return resp
}

type fuzzRequest struct {
	Form     url.Values
	PostForm url.Values
	Body     string
	Headers  map[string][]string
}

func newFuzzRequest() *fuzzRequest {
	return &fuzzRequest{}
}

func (s *fuzzRequest) Fuzz(t *testing.T, data []byte, method, addr string) *http.Request {
	t.Helper()

	fuzzConsumer := fuzz.NewConsumer(data)
	if err := fuzzConsumer.GenerateStruct(s); err != nil {
		t.Skip(err) // not enough data for fuzzing
	}
	bodyReader := new(bytes.Buffer)

	enc := json.NewEncoder(bodyReader)
	if err := enc.Encode(io.NopCloser(strings.NewReader(s.Body))); err != nil {
		t.Fatalf("err:%s", err)
	}

	req, err := http.NewRequest(method, addr, bodyReader)
	if err != nil {
		t.Skipf("Skipping test: not enough data for fuzzing: %s", err.Error())
	}

	// Get the address of the local listener in order to attach it to an Origin header.
	// This will allow for the testing of requests that require CORS, without using a browser.
	hostURLRegexp := regexp.MustCompile("http[s]?://.+:[0-9]+")

	for k, v := range s.Headers {
		for i := range v {
			req.Header.Add(k, v[i])
		}
	}
	req.Header.Set("Origin", hostURLRegexp.FindString(addr))
	req.Header.Set("Content-Length", strconv.Itoa(len(s.Body)))
	req.Header.Set("Content-Type", "application/octet-stream")

	req.Form = s.Form
	req.PostForm = s.PostForm

	return req
}

func GetPortFromEnv(env string) (port int, err error) {
	portEnv := os.Getenv(env)
	if portEnv == "" {
		return 0, fmt.Errorf("port env var %s not set", env)
	}

	port, err = strconv.Atoi(portEnv)
	if err != nil {
		return 0, fmt.Errorf("failed to parse port env var %s: %w", env, err)
	}
	return port, nil
}
