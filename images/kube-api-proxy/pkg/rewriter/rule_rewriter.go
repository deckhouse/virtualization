package rewriter

import (
	"fmt"
	"io"
	log "log/slog"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

type RewriteRequestResult struct {
	IsWatch          bool
	IsWebhook        bool
	OrigGroup        string
	OrigResourceType string
	PathItems        []string
	TargetPath       string
	Body             []byte
}

type RuleBasedRewriter struct {
	Rules *RewriteRules
	// ToTargetMode is a rewrite mode from client to target.
	ToTargetMode Mode
	// FromTargetMode
	FromTargetMode Mode
}

type Mode string

const (
	// Restore is a rewrite to restore resources to original.
	Restore Mode = "restore"
	// Rename is a rewrite to rename original resources.
	Rename Mode = "rename"
)

// RewriteToTarget rewrites path and body in the request. Argument mode defines
// either rewriter should rename resources if request is from the client, or
// restore resources if it is a call from the API Server to the webhook.
func (rw *RuleBasedRewriter) RewriteToTarget(req *http.Request) (*RewriteRequestResult, error) {
	if req == nil || req.URL == nil {
		return nil, nil
	}

	res, err := rw.RewritePath(req.URL.Path)
	if err != nil {
		return nil, err
	}

	if res == nil {
		res = &RewriteRequestResult{}
	}

	if strings.Contains(req.URL.RawQuery, "watch=true") {
		res.IsWatch = true
	}

	// GET requests have no payload to rewrite.
	if req.Method == http.MethodGet {
		return res, nil
	}

	// Read whole body to rewrite.
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}

	res.Body, err = rw.rewriteBodyJSON(res, bodyBytes, rw.ToTargetMode)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// RewriteFromTarget rewrites payloads in responses from the target.
// TODO support rewriting of protobuf object, e.g. Pods to fix ownerRefs.
func (rw *RuleBasedRewriter) RewriteFromTarget(reqResult *RewriteRequestResult, contentType string, body io.ReadCloser) ([]byte, error) {
	// Rewrite supports only json responses for now.
	if !strings.HasPrefix(contentType, "application/json") {
		return nil, nil
	}

	// Read whole body to rewrite.
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	return rw.rewriteBodyJSON(reqResult, bodyBytes, rw.FromTargetMode)
}

func (rw *RuleBasedRewriter) rewriteBodyJSON(reqResult *RewriteRequestResult, obj []byte, mode Mode) ([]byte, error) {
	// Detect Kind
	kind := gjson.GetBytes(obj, "kind").String()
	log.Info(fmt.Sprintf("Got kind %s", kind))

	var rwrBytes []byte
	var err error

	switch kind {
	case "APIGroupList":
		rwrBytes, err = RewriteAPIGroupList(rw.Rules, obj)

	case "APIGroup":
		rwrBytes, err = RewriteAPIGroup(rw.Rules, obj, reqResult.OrigGroup)

	case "APIResourceList":
		rwrBytes, err = RewriteAPIResourceList(rw.Rules, obj, reqResult.OrigGroup)

	case "AdmissionReview":
		rwrBytes, err = RewriteAdmissionReview(rw.Rules, obj, reqResult.OrigGroup)

	//case "Pod":
	// rewrite owner

	default:
		rwrBytes, err = RewriteResourceOrList(rw.Rules, obj, mode)
	}

	// Return obj bytes as-is in case of the error.
	if err != nil {
		return obj, err
	}

	return rwrBytes, nil
}
