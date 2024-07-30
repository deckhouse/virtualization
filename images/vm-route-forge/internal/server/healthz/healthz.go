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
