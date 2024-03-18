package proxy

import (
	"bytes"
	"fmt"
	"io"
	log "log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	logutil "kube-api-proxy/pkg/log"
	"kube-api-proxy/pkg/rewriter"
)

type Handler struct {
	Name string
	// ProxyPass is a target http client to send requests to.
	// An allusion to nginx proxy_pass directive.
	TargetClient *http.Client
	TargetURL    *url.URL
	Rewriter     *rewriter.RuleBasedRewriter
	m            sync.Mutex
}

func (p *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	log := log.With(log.String("proxy.name", p.Name))

	//isWatch := strings.Contains(req.URL.RawQuery, "watch=true")
	//if !isWatch {
	//	// Debug: lock handlers for readable logs.
	//	p.m.Lock()
	//	defer p.m.Unlock()
	//}

	log.Info(fmt.Sprintf("%s %s %s", req.Method, req.RemoteAddr, req.URL.String()))

	req.RequestURI = ""
	req.URL.Scheme = p.TargetURL.Scheme
	req.URL.Host = p.TargetURL.Host

	//origPath := req.URL.Path
	origURL := req.URL.String()

	// Rewrite host and query and detect if rewrite is needed.
	reqResult, err := p.Rewriter.RewriteToTarget(req)
	if err != nil {
		log.Error(fmt.Sprintf("Error rewriting request: %s", req.URL.String()), logutil.SlogErr(err))
		http.Error(w, "can't rewrite request", http.StatusBadRequest)
		return
	}

	if reqResult == nil {
		log.Info(fmt.Sprintf("%s %s %s", req.Method, req.RemoteAddr, origURL))
	} else {
		// Rewrite protobuf to json to rewrite watch events with core resources.
		// TODO check resource name in path and override content-type for particular resources only (i.e. Pods).
		if reqResult.IsWatch && reqResult.IsCoreAPI {
			newAccept := make([]string, 0)
			for _, hdr := range req.Header.Values("Accept") {
				if strings.Contains(hdr, "application/vnd.kubernetes.protobuf") {
					newAccept = append(newAccept, "application/json")
				} else {
					newAccept = append(newAccept, hdr)
				}
			}
			req.Header["Accept"] = newAccept
		}
		if reqResult.TargetPath != "" {
			req.URL.Path = reqResult.TargetPath
			log.Info(fmt.Sprintf("%s %s %s -> %s", req.Method, req.RemoteAddr, origURL, req.URL.String()))
		} else {
			log.Info(fmt.Sprintf("%s %s %s", req.Method, req.RemoteAddr, req.URL.String()))
		}
		if len(reqResult.Body) > 0 {
			// Fix content-length if needed.
			log.Info(fmt.Sprintf("request headers: %+v", req.Header))
			req.ContentLength = int64(len(reqResult.Body))
			if req.Header.Get("Content-Length") != "" {
				req.Header.Set("Content-Length", strconv.Itoa(len(reqResult.Body)))
			}
			req.Body = io.NopCloser(bytes.NewBuffer(reqResult.Body))
		}
	}
	log.Info(fmt.Sprintf("request headers: %+v", req.Header))

	// Wrap reader to log content after transferring request Body.
	req.Body = logutil.NewReaderLogger(req.Body)

	resp, err := p.TargetClient.Do(req)
	if err != nil {
		log.Error("Proxy pass request error", logutil.SlogErr(err))
		http.Error(w, "Proxy pass request error", http.StatusInternalServerError)
		// TODO return apimachinery NewInternalError
		// https://github.com/kubernetes/apimachinery/blob/master/pkg/api/errors/errors.go
		return
	}
	defer resp.Body.Close()

	// Log head of the request body.
	log.Info(fmt.Sprintf("request Body: %s", logutil.HeadStringEx(req.Body, 300)))

	if reqResult == nil {
		// Pass response as-is and without logging.
		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		return
	}

	// Watch rewriter  does rewrites for every event in background.
	if reqResult.IsWatch {
		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)

		log.Info(fmt.Sprintf("Start streaming watch events. Response: Status %s, Content-Type %s", resp.Status, resp.Header.Get("Content-Type")))
		// Start stream handler and lock ServeHTTP while proxying watch events stream.
		wsr, err := NewStreamHandler(resp.Body, w, resp.Header.Get("Content-Type"), p.Rewriter, reqResult)
		if err != nil {
			log.Error("Error watching stream", logutil.SlogErr(err))
			http.Error(w, fmt.Sprintf("watch stream: %v", err), http.StatusInternalServerError)
			return
		}
		<-wsr.DoneChan()
		return
	}

	// One-time rewrite is required for client or webhook requests.
	log.Info(fmt.Sprintf("Rewrite response: Status %s, Content-Type %s", resp.Status, resp.Header.Get("Content-Type")))

	bodyBytes, err := p.Rewriter.RewriteFromTarget(reqResult, resp.Header.Get("Content-Type"), resp.Body)
	if err != nil {
		log.Error("Error rewriting response", logutil.SlogErr(err))
		http.Error(w, "can't rewrite response", http.StatusBadRequest)
		return
	}
	// Fix Content-Length header.
	if bodyBytes != nil {
		w.Header().Set("Content-Length", strconv.Itoa(len(bodyBytes)))
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	// Wrap reader to log content after transferring response Body.
	resp.Body = logutil.NewReaderLogger(resp.Body)

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	// Log body beginning.
	log.Info(fmt.Sprintf("response Body: %s", logutil.HeadStringEx(resp.Body, 300)))
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
