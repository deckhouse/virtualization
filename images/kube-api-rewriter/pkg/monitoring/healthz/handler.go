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

package healthz

import "net/http"

// AddHealthzHandler adds endpoints for health and readiness probes.
func AddHealthzHandler(mux *http.ServeMux) {
	if mux == nil {
		return
	}
	mux.HandleFunc("/healthz", okStatusHandler)
	mux.HandleFunc("/healthz/", okStatusHandler)
	mux.HandleFunc("/readyz", okStatusHandler)
	mux.HandleFunc("/readyz/", okStatusHandler)
}

func okStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
