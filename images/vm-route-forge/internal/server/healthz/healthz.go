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

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/emicklei/go-restful/v3"
	"k8s.io/client-go/kubernetes"
)

func NewHandler(client kubernetes.Interface) *Handler {
	return &Handler{client: client}
}

type Handler struct {
	client kubernetes.Interface
}

func (h Handler) WebService() *restful.WebService {
	webService := new(restful.WebService)
	webService.Path("/").Consumes(restful.MIME_JSON).Produces(restful.MIME_JSON)
	webService.Route(webService.GET("/healthz").To(h.handle).Doc("Health endpoint"))
	return webService
}

func (h Handler) handle(_ *restful.Request, response *restful.Response) {
	body, err := h.client.
		CoreV1().
		RESTClient().
		Get().
		AbsPath("/version").
		Do(context.Background()).
		Raw()
	if err != nil {
		h.unhealthy(err, response)
		return
	}
	var version interface{}
	err = json.Unmarshal(body, &version)
	if err != nil {
		h.unhealthy(err, response)
		return
	}
	res := map[string]interface{}{"connectivity": "ok", "version": version}
	response.WriteHeaderAndJson(http.StatusOK, res, restful.MIME_JSON)
}

func (h Handler) unhealthy(err error, response *restful.Response) {
	res := map[string]interface{}{"connectivity": "failed", "error": fmt.Sprintf("%v", err)}
	response.WriteHeaderAndJson(http.StatusInternalServerError, res, restful.MIME_JSON)
}
