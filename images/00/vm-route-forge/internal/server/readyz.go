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

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

func (s *Server) getReadyzHandler() http.Handler {
	if s.readyzHandler != nil {
		return s.readyzHandler
	}
	unhealthy := func(err error, w http.ResponseWriter) {
		res := map[string]interface{}{"connectivity": "failed", "error": fmt.Sprintf("%v", err)}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(res)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := s.client.
			CoreV1().
			RESTClient().
			Get().
			AbsPath("/version").
			Do(context.Background()).
			Raw()
		if err != nil {
			unhealthy(err, w)
			return
		}
		var version interface{}
		err = json.Unmarshal(body, &version)
		if err != nil {
			unhealthy(err, w)
			return
		}
		res := map[string]interface{}{"connectivity": "ok", "version": version}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(res)

	})
}
