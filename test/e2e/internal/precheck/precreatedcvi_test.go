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

package precheck

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deckhouse/virtualization-controller/pkg/builder/cvi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestProbeHTTPSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method is %s, want HEAD", r.Method)
		}
		if r.URL.Path == "/missing.raw" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Length", "12345")
	}))
	defer srv.Close()

	present := cvi.New(cvi.WithName("present"), cvi.WithDataSourceHTTP(srv.URL+"/present.qcow2", nil, nil))
	size, err := probeHTTPSource(t.Context(), present)
	if err != nil || size != "12345" {
		t.Errorf("present image: size %q, error %v, want 12345 and no error", size, err)
	}

	missing := cvi.New(cvi.WithName("missing"), cvi.WithDataSourceHTTP(srv.URL+"/missing.raw", nil, nil))
	_, err = probeHTTPSource(t.Context(), missing)
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") || !strings.Contains(err.Error(), "/missing.raw") {
		t.Errorf("missing image: error %v, want one naming the URL and HTTP 404", err)
	}

	if size, err := probeHTTPSource(t.Context(), cvi.New(cvi.WithName("registry"))); err != nil || size != "" {
		t.Errorf("non-HTTP source: size %q, error %v, want neither", size, err)
	}
}

func TestIsStale(t *testing.T) {
	desired := cvi.New(cvi.WithName("img"), cvi.WithDataSourceHTTP("https://images/custom-bios.qcow2", nil, nil))
	current := func(size string) *v1alpha2.ClusterVirtualImage {
		existing := desired.DeepCopy()
		existing.Status.Phase = v1alpha2.ImageReady
		if size != "" {
			existing.SetAnnotations(map[string]string{annSourceSize: size})
		}
		return existing
	}

	if isStale(current("100"), desired, "100") {
		t.Error("same URL and size must not be stale")
	}
	if isStale(current("100"), desired, "") {
		t.Error("unknown size must not be stale")
	}
	if !isStale(current("100"), desired, "200") {
		t.Error("changed size must be stale")
	}
	if !isStale(current(""), desired, "100") {
		t.Error("unrecorded size must be stale")
	}
	other := cvi.New(cvi.WithName("img"), cvi.WithDataSourceHTTP("https://images/custom.qcow2", nil, nil))
	if !isStale(other, desired, "100") {
		t.Error("changed URL must be stale")
	}
	lost := current("100")
	lost.Status.Phase = v1alpha2.ImageLost
	if !isStale(lost, desired, "100") {
		t.Error("lost image must be stale")
	}
}
