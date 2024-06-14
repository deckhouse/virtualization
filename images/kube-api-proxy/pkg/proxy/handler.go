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

	// Step 1. Parse request url, prepare path rewrite.
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
	if targetReq.Path() != req.URL.Path || targetReq.RawQuery() != req.URL.RawQuery {
		logger.Info(fmt.Sprintf("%s [%s,%s] %s -> %s", req.Method, rwrReq, rwrResp, req.URL.RequestURI(), targetReq.RequestURI()))
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

	// Step 2. Modify request endpoint, headers and body bytes before send it to the target.
	origRequestBytes, rwrRequestBytes, err := h.transformRequest(targetReq, req)
	if err != nil {
		logger.Error(fmt.Sprintf("Error transforming request: %s", req.URL.String()), logutil.SlogErr(err))
		http.Error(w, "can't rewrite request", http.StatusBadRequest)
		return
	}

	logger.Info(fmt.Sprintf("Request: target headers: %+v", req.Header))

	// Restore req.Body as this reader was read earlier by the transformRequest.
	if rwrRequestBytes != nil {
		req.Body = io.NopCloser(bytes.NewBuffer(rwrRequestBytes))
	} else if origRequestBytes != nil {
		// Fallback to origRequestBytes if body was not rewritten.
		req.Body = io.NopCloser(bytes.NewBuffer(origRequestBytes))
	}

	// Step 3. Send request to the target.
	resp, err := h.TargetClient.Do(req)
	if err != nil {
		logger.Error("Error passing request to the target", logutil.SlogErr(err))
		http.Error(w, "Error passing request to the target", http.StatusInternalServerError)
		// TODO return apimachinery NewInternalError
		// https://github.com/kubernetes/apimachinery/blob/master/pkg/api/errors/errors.go
		return
	}
	defer resp.Body.Close()

	// TODO handle resp.Status 3xx, 4xx, 5xx, etc.

	if req.Method == http.MethodPatch {
		logutil.DebugBodyHead(logger, "Request PATCH", "patch", origRequestBytes)
		if len(rwrRequestBytes) > 0 {
			logutil.DebugBodyChanges(logger, "Request PATCH", "patch", origRequestBytes, rwrRequestBytes)
		}
	} else {
		logutil.DebugBodyChanges(logger, "Request", resource, origRequestBytes, rwrRequestBytes)
	}

	// Step 5. Handle response: pass through, transform resp.Body, or run stream transformer.

	if !targetReq.ShouldRewriteResponse() {
		// Pass response as-is without rewriting.
		if targetReq.IsWatch() {
			logger.Debug(fmt.Sprintf("Response decision: PASS STREAM, Status %s, Headers %+v", resp.Status, resp.Header))
		} else {
			logger.Debug(fmt.Sprintf("Response decision: PASS, Status %s, Headers %+v", resp.Status, resp.Header))
		}
		passResponse(targetReq, w, resp, logger)
		return
	}

	if targetReq.IsWatch() {
		logger.Debug(fmt.Sprintf("Response decision: REWRITE STREAM, Status %s, Headers %+v", resp.Status, resp.Header))

		h.transformStream(targetReq, w, resp, logger)
		return
	}

	// One-time rewrite is required for client or webhook requests.
	logger.Debug(fmt.Sprintf("Response decision: REWRITE, Status %s, Headers %+v", resp.Status, resp.Header))

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
func (h *Handler) transformRequest(targetReq *rewriter.TargetRequest, req *http.Request) ([]byte, []byte, error) {
	if req == nil || req.URL == nil {
		return nil, nil, fmt.Errorf("http request and URL should not be nil")
	}

	var origBodyBytes []byte
	var rwrBodyBytes []byte
	var err error

	hasPayload := req.Body != nil

	if hasPayload {
		origBodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, nil, fmt.Errorf("read request body: %w", err)
		}
	}

	// Rewrite incoming payload, e.g. create, put, etc.
	if targetReq.ShouldRewriteRequest() && hasPayload {
		switch req.Method {
		case http.MethodPatch:
			rwrBodyBytes, err = h.Rewriter.RewritePatch(targetReq, origBodyBytes)
		default:
			rwrBodyBytes, err = h.Rewriter.RewriteJSONPayload(targetReq, origBodyBytes, ToTargetAction(h.ProxyMode))
		}
		if err != nil {
			return nil, nil, err
		}

		// Put new Body reader to req and fix Content-Length header.
		rwrBodyLen := len(rwrBodyBytes)
		if rwrBodyLen > 0 {
			// Fix content-length if needed.
			req.ContentLength = int64(rwrBodyLen)
			if req.Header.Get("Content-Length") != "" {
				req.Header.Set("Content-Length", strconv.Itoa(rwrBodyLen))
			}
			//req.Body = io.NopCloser(bytes.NewBuffer(newBody))
		}
	}

	// TODO Implement protobuf and table rewriting to remove these manipulations with Accept header.
	// TODO Move out to a separate method forceApplicationJSONContent.
	if targetReq.ShouldRewriteResponse() {
		newAccept := make([]string, 0)
		for _, hdr := range req.Header.Values("Accept") {
			// Rewriter doesn't work with protobuf, force JSON in Accept header.
			// This workaround is suitable only for empty body requests: Get, List, etc.
			// A client should be patched to send JSON requests.
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

	return origBodyBytes, rwrBodyBytes, nil
}

func passResponse(targetReq *rewriter.TargetRequest, w http.ResponseWriter, resp *http.Response, logger *slog.Logger) {
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	dst := &immediateWriter{dst: w}

	if logger.Enabled(nil, slog.LevelDebug) {
		if targetReq.IsWatch() {
			dst.chunkFn = func(chunk []byte) {
				logger.Debug(fmt.Sprintf("Pass through response chunk: %s", string(chunk)))
			}
		} else {
			resp.Body = logutil.NewReaderLogger(resp.Body)
		}
	}

	_, err := io.Copy(dst, resp.Body)
	if err != nil {
		logger.Error(fmt.Sprintf("copy target response back to client: %v", err))
	}

	if logger.Enabled(nil, slog.LevelDebug) && !targetReq.IsWatch() {
		logutil.DebugBodyHead(logger,
			fmt.Sprintf("Pass through response: status %d, content-length: '%s'", resp.StatusCode, resp.Header.Get("Content-Length")),
			"",
			logutil.Bytes(resp.Body),
		)
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
		passResponse(targetReq, w, resp, logger)
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

	// Step 1. Read response body to buffer.
	origBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Error reading response payload", logutil.SlogErr(err))
		http.Error(w, "Error reading response payload", http.StatusBadGateway)
		return
	}

	// Step 2. Rewrite response JSON.
	rwrBodyBytes, err := h.Rewriter.RewriteJSONPayload(targetReq, origBodyBytes, FromTargetAction(h.ProxyMode))
	if err != nil {
		logger.Error("Error rewriting response", logutil.SlogErr(err))
		http.Error(w, "can't rewrite response", http.StatusInternalServerError)
		return
	}

	logutil.DebugBodyChanges(logger, "Response", targetReq.OrigResourceType(), origBodyBytes, rwrBodyBytes)

	// Step 3. Fix headers before sending response back to the client.
	copyHeader(w.Header(), resp.Header)
	// Fix Content headers.
	// rwrBodyBytes are always decoded from gzip. Delete header to not break our client.
	w.Header().Del("Content-Encoding")
	if rwrBodyBytes != nil {
		w.Header().Set("Content-Length", strconv.Itoa(len(rwrBodyBytes)))
	}
	w.WriteHeader(resp.StatusCode)

	// Step 4. Write non-empty rewritten body to the client.
	if rwrBodyBytes != nil {
		resp.Body = io.NopCloser(bytes.NewBuffer(rwrBodyBytes))
		_, err := io.Copy(w, resp.Body)
		if err != nil {
			logger.Error(fmt.Sprintf("error writing response from target to the client: %v", err))
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
	dst     io.Writer
	chunkFn func([]byte)
}

func (iw *immediateWriter) Write(p []byte) (n int, err error) {
	n, err = iw.dst.Write(p)

	if iw.chunkFn != nil {
		iw.chunkFn(p)
	}

	if flusher, ok := iw.dst.(http.Flusher); ok {
		flusher.Flush()
	}

	return
}
