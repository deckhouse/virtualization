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

package proxy

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	logutil "kube-api-proxy/pkg/log"
	"kube-api-proxy/pkg/rewriter"
)

type ProxyMode string

const (
	// ToOriginal mode indicates that resource should be restored when passed to target and renamed when passing back to client.
	ToOriginal ProxyMode = "original"
	// ToRenamed mode indicates that resource should be renamed when passed to target and restored when passing back to client.
	ToRenamed ProxyMode = "renamed"
)

func ToTargetAction(proxyMode ProxyMode) rewriter.Action {
	if proxyMode == ToRenamed {
		return rewriter.Rename
	}
	return rewriter.Restore
}

func FromTargetAction(proxyMode ProxyMode) rewriter.Action {
	if proxyMode == ToRenamed {
		return rewriter.Restore
	}
	return rewriter.Rename
}

type Handler struct {
	Name string
	// ProxyPass is a target http client to send requests to.
	// An allusion to nginx proxy_pass directive.
	TargetClient *http.Client
	TargetURL    *url.URL
	ProxyMode    ProxyMode
	Rewriter     *rewriter.RuleBasedRewriter
	m            sync.Mutex
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req == nil {
		slog.Error("req is nil. something wrong")
		return
	}
	if req.URL == nil {
		slog.Error(fmt.Sprintf("req.URL is nil. something wrong. method %s RequestURI '%s' Headers %+v", req.Method, req.RequestURI, req.Header))
		return
	}

	// Parse request url, prepare path rewrite.
	targetReq := rewriter.NewTargetRequest(h.Rewriter, req)

	resource := targetReq.ResourceForLog()

	logger := slog.With(
		slog.String("request", fmt.Sprintf("%s %s", req.Method, req.URL.Path)),
		slog.String("resource", resource),
		slog.String("proxy.name", h.Name),
	)

	// Set target address, cleanup RequestURI.
	req.RequestURI = ""
	req.URL.Scheme = h.TargetURL.Scheme
	req.URL.Host = h.TargetURL.Host

	// Log request path.
	rwrReq := " NO"
	if targetReq.ShouldRewriteRequest() {
		rwrReq = "REQ"
	}
	rwrResp := "  NO"
	if targetReq.ShouldRewriteResponse() {
		rwrResp = "RESP"
	}
	if targetReq.Path() != req.URL.Path {
		logger.Info(fmt.Sprintf("%s [%s,%s] %s -> %s", req.Method, rwrReq, rwrResp, req.URL.String(), targetReq.Path()))
	} else {
		logger.Info(fmt.Sprintf("%s [%s,%s] %s", req.Method, rwrReq, rwrResp, req.URL.String()))
	}

	// TODO(development): Mute some logging for development: election, non-rewritable resources.
	isMute := false
	if !targetReq.ShouldRewriteRequest() && !targetReq.ShouldRewriteResponse() {
		isMute = true
	}
	switch resource {
	case "leases":
		isMute = true
	case "endpoints":
		isMute = true
	case "clusterrolebindings":
		isMute = false
	case "clustervirtualmachineimages":
		isMute = false
	}
	if isMute {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	logger.Info(fmt.Sprintf("Request: orig headers: %+v", req.Header))

	// Modify req to send it to target.
	err := h.transformRequest(targetReq, req)
	//targetReq, err := p.Rewriter.RewriteToTarget(req)
	if err != nil {
		logger.Error(fmt.Sprintf("Error rewriting request: %s", req.URL.String()), logutil.SlogErr(err))
		http.Error(w, "can't rewrite request", http.StatusBadRequest)
		return
	}

	logger.Info(fmt.Sprintf("Request: target headers: %+v", req.Header))

	// Wrap reader to log content after transferring request Body.
	if req.Body != nil {
		req.Body = logutil.NewReaderLogger(req.Body)
	}

	resp, err := h.TargetClient.Do(req)
	if err != nil {
		logger.Error("Proxy pass request error", logutil.SlogErr(err))
		http.Error(w, "Proxy pass request error", http.StatusInternalServerError)
		// TODO return apimachinery NewInternalError
		// https://github.com/kubernetes/apimachinery/blob/master/pkg/api/errors/errors.go
		return
	}
	defer resp.Body.Close()

	// TODO handle resp.Status 3xx, 4xx, 5xx, etc.

	// TODO delete after development: Log head of the request body.
	if logutil.HasData(req.Body) {
		limit := 512
		switch resource {
		case "virtualmachines",
			"virtualmachines/status",
			"virtualmachineinstances",
			"virtualmachineinstances/status",
			"clustervirtualmachineimages",
			"clustervirtualmachineimages/status",
			"clusterrolebindings",
			"customresourcedefinitions":
			limit = 32000
		}
		logger.Info(fmt.Sprintf("Request: Rewritten body: %s", logutil.HeadStringEx(req.Body, limit)))
	}

	if !targetReq.ShouldRewriteResponse() {
		// Pass response as-is without rewriting.
		logger.Info(fmt.Sprintf("RESPONSE PASS: Status %s, Headers %+v", resp.Status, resp.Header))
		passResponse(w, resp, logger)
		return
	}

	if targetReq.IsWatch() {
		logger.Info(fmt.Sprintf("RESPONSE REWRITE STREAM Status %s, Headers %+v", resp.Status, resp.Header))

		h.transformStream(targetReq, w, resp, logger)
		return
	}

	// One-time rewrite is required for client or webhook requests.
	logger.Info(fmt.Sprintf("RESPONSE REWRITE ONCE Status %s, Headers %+v", resp.Status, resp.Header))

	h.transformResponse(targetReq, w, resp, logger)
	return
}

func copyHeader(dst, src http.Header) {
	for header, values := range src {
		// Do not override dst header with the header from the src.
		if len(dst.Values(header)) > 0 {
			continue
		}
		for _, value := range values {
			dst.Add(header, value)
		}
	}
}

func encodingAwareBodyReader(resp *http.Response) (io.ReadCloser, error) {
	if resp == nil {
		return nil, nil
	}
	body := resp.Body
	if body == nil {
		return nil, nil
	}

	var reader io.ReadCloser
	var err error

	encoding := resp.Header.Get("Content-Encoding")
	switch encoding {
	case "gzip":
		reader, err = gzip.NewReader(body)
		if err != nil {
			return nil, fmt.Errorf("errorf making gzip reader: %v", err)
		}
		return io.NopCloser(reader), nil
	case "deflate":
		return flate.NewReader(body), nil
	}

	return body, nil
}

// transformRequest transforms request headers and rewrites request payload to use
// request as client to the target.
// TargetMode field defines either transformer should rename resources
// if request is from the client, or restore resources if it is a call
// from the API Server to the webhook.
func (h *Handler) transformRequest(targetReq *rewriter.TargetRequest, req *http.Request) error {
	if req == nil || req.URL == nil {
		return fmt.Errorf("request to rewrite is empty")
	}

	// Rewrite incoming payload, e.g. create, put, etc.
	if targetReq.ShouldRewriteRequest() && req.Body != nil {
		// Read whole request body to rewrite.
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("read request body: %w", err)
		}

		var newBody []byte
		switch req.Method {
		case http.MethodPatch:
			newBody, err = h.Rewriter.RewritePatch(targetReq, bodyBytes)
		default:
			newBody, err = h.Rewriter.RewriteJSONPayload(targetReq, bodyBytes, ToTargetAction(h.ProxyMode))
		}
		if err != nil {
			return err
		}

		// Put new Body reader to req and fix Content-Length header.
		newBodyLen := len(newBody)
		if newBodyLen > 0 {
			// Fix content-length if needed.
			req.ContentLength = int64(newBodyLen)
			if req.Header.Get("Content-Length") != "" {
				req.Header.Set("Content-Length", strconv.Itoa(newBodyLen))
			}
			req.Body = io.NopCloser(bytes.NewBuffer(newBody))
		}
	}

	if targetReq.ShouldRewriteResponse() {
		// Rewriter not work with protobuf, force JSON
		// in Accept header.
		newAccept := make([]string, 0)
		for _, hdr := range req.Header.Values("Accept") {
			if strings.Contains(hdr, "application/vnd.kubernetes.protobuf") {
				newAccept = append(newAccept, "application/json")
				continue
			}

			// TODO Add rewriting support for Table format.
			// Quickly support kubectl with simple hack
			if strings.Contains(hdr, "application/json") && strings.Contains(hdr, "as=Table") {
				newAccept = append(newAccept, "application/json")
				continue
			}

			newAccept = append(newAccept, hdr)
		}
		//req.Header.Set("Accept", newAccept)

		//req.Header.Del("Accept")
		req.Header["Accept"] = newAccept

		// Force JSON for watches of core resources and CRDs.
		if targetReq.IsWatch() && (targetReq.IsCRD() || targetReq.IsCore()) {
			if len(req.Header.Values("Accept")) == 0 {
				req.Header["Accept"] = []string{"application/json"}
			}
		}
	}

	// Set new endpoint path and query.
	req.URL.Path = targetReq.Path()
	req.URL.RawQuery = targetReq.RawQuery()

	return nil
}

func passResponse(w http.ResponseWriter, resp *http.Response, logger *slog.Logger) {
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		resp.Body = logutil.NewReaderLogger(resp.Body)
	}

	dst := &immediateWriter{dst: w}
	_, err := io.Copy(dst, resp.Body)
	if err != nil {
		logger.Error(fmt.Sprintf("copy response: %v", err))
	}

	if logutil.HasData(resp.Body) {
		limit := 1024
		logger.Info(fmt.Sprintf("Pass through non 200 response: status %d %s", resp.StatusCode, logutil.HeadStringEx(resp.Body, limit)))
	}

	return
}

// transformResponse rewrites payloads in responses from the target.
//
// ProxyMode field defines either rewriter should restore, or rename resources.
func (h *Handler) transformResponse(targetReq *rewriter.TargetRequest, w http.ResponseWriter, resp *http.Response, logger *slog.Logger) {
	// Rewrite supports only json responses for now.
	// TODO detect content type from content, header in response may be inaccurate, e.g. from webhooks.
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		logger.Warn(fmt.Sprintf("Will not transform non JSON response from target: Content-type=%s", contentType))
		passResponse(w, resp, logger)
		return
	}

	// Add gzip decoder if needed.
	var err error
	resp.Body, err = encodingAwareBodyReader(resp)
	if err != nil {
		logger.Error("Error decoding response body", logutil.SlogErr(err))
		http.Error(w, "can't decode response body", http.StatusInternalServerError)
		return
	}

	// Read response body to buffer. Wrap Body to log it later.
	bodyReader := logutil.NewReaderLogger(resp.Body)
	bodyBytes, err := io.ReadAll(bodyReader)
	if err != nil {
		logger.Error("Error reading response payload", logutil.SlogErr(err))
		http.Error(w, "Error reading response payload", http.StatusBadGateway)
		return
	}

	{
		limit := 1024
		logger.Info(fmt.Sprintf("Response: original body: [%d] %s", len(bodyBytes), logutil.HeadStringEx(bodyReader, limit)))
	}

	//return tr.Rewriter.RewriteJSONPayload(tr.TargetReq, bodyBytes, FromTargetAction(tr.ProxyMode))
	//bodyBytes, err := h.Rewriter.RewriteFromTarget(targetReq, resp, logger)
	bodyBytes, err = h.Rewriter.RewriteJSONPayload(targetReq, bodyBytes, FromTargetAction(h.ProxyMode))
	if err != nil {
		logger.Error("Error rewriting response", logutil.SlogErr(err))
		http.Error(w, "can't rewrite response", http.StatusInternalServerError)
		return
	}

	{
		limit := 1024
		if len(bodyBytes) < limit {
			limit = len(bodyBytes)
		}
		logger.Info(fmt.Sprintf("Response: rewritten bytes: [%d] %s", len(bodyBytes), string(bodyBytes[:limit])))
	}

	copyHeader(w.Header(), resp.Header)
	// Fix Content headers.
	// bodyBytes are always decoded from gzip. Delete header to not break our client.
	w.Header().Del("Content-Encoding")
	if bodyBytes != nil {
		w.Header().Set("Content-Length", strconv.Itoa(len(bodyBytes)))
	}
	w.WriteHeader(resp.StatusCode)

	if bodyBytes != nil {
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		_, err := io.Copy(w, resp.Body)
		if err != nil {
			logger.Error(fmt.Sprintf("error writing response from target: %v", err))
		}
	}

	return
}

func (h *Handler) transformStream(targetReq *rewriter.TargetRequest, w http.ResponseWriter, resp *http.Response, logger *slog.Logger) {
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	// Start stream handler and lock ServeHTTP while proxying watch events stream.
	wsr, err := NewStreamHandler(resp.Body, w, resp.Header.Get("Content-Type"), h.Rewriter, targetReq, logger)
	if err != nil {
		logger.Error("Error watching stream", logutil.SlogErr(err))
		http.Error(w, fmt.Sprintf("watch stream: %v", err), http.StatusInternalServerError)
		return
	}
	<-wsr.DoneChan()
	return
}

type immediateWriter struct {
	dst io.Writer
}

func (iw *immediateWriter) Write(p []byte) (n int, err error) {
	n, err = iw.dst.Write(p)

	if flusher, ok := iw.dst.(http.Flusher); ok {
		flusher.Flush()
	}

	return
}
