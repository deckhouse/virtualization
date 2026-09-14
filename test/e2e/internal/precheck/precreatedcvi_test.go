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
)

func TestCheckImageURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method is %s, want HEAD", r.Method)
		}
		if r.URL.Path == "/missing.raw" {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	present := cvi.New(cvi.WithName("present"), cvi.WithDataSourceHTTP(srv.URL+"/present.qcow2", nil, nil))
	if err := checkImageURL(t.Context(), present); err != nil {
		t.Errorf("present image: unexpected error: %v", err)
	}

	missing := cvi.New(cvi.WithName("missing"), cvi.WithDataSourceHTTP(srv.URL+"/missing.raw", nil, nil))
	err := checkImageURL(t.Context(), missing)
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") || !strings.Contains(err.Error(), "/missing.raw") {
		t.Errorf("missing image: error %v, want one naming the URL and HTTP 404", err)
	}

	if err := checkImageURL(t.Context(), cvi.New(cvi.WithName("registry"))); err != nil {
		t.Errorf("non-HTTP source: unexpected error: %v", err)
	}
}
