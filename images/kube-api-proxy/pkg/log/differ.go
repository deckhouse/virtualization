package log

import (
	"bytes"
	"fmt"
	"log/slog"

	jd "github.com/josephburnett/jd/lib"
	"github.com/tidwall/gjson"
)

// DebugBodyChanges logs debug message with diff between 2 bodies.
func DebugBodyChanges(logger *slog.Logger, msg string, resourceType string, inBytes, rwrBytes []byte) {
	if !logger.Enabled(nil, slog.LevelDebug) {
		return
	}

	if len(inBytes) == 0 && len(rwrBytes) == 0 {
		logger.Debug(fmt.Sprintf("%s: empty body", msg))
		return
	}

	if len(inBytes) == 0 && len(rwrBytes) != 0 {
		logger.Debug(fmt.Sprintf("%s: possible bug: empty body produces %d bytes", msg, len(rwrBytes)))
		DebugBodyHead(logger, msg, resourceType, rwrBytes)
		return
	}

	if len(inBytes) != 0 && len(rwrBytes) == 0 {
		logger.Error(fmt.Sprintf("%s: non-empty body [%d] produces empty rewrite", msg, len(inBytes)))
		DebugBodyHead(logger, msg, resourceType, inBytes)
		return
	}

	// Print diff for non-empty non-equal JSONs.
	diffContent, err := Diff(inBytes, rwrBytes)
	if err != nil {
		// Rollback to printing a limited part of the JSON.
		logger.Error(fmt.Sprintf("Can't diff '%s' JSONs after rewrite", resourceType), SlogErr(err))
		DebugBodyHead(logger, msg, resourceType, rwrBytes)
		return
	}

	// TODO pass ns/name as arguments for patches.
	apiVersion := gjson.GetBytes(inBytes, "apiVersion")
	kind := gjson.GetBytes(inBytes, "kind")
	ns := gjson.GetBytes(inBytes, "metadata.namespace")
	name := gjson.GetBytes(inBytes, "metadata.name")
	logger.Debug(fmt.Sprintf("%s: changes after rewrite for %s/%s/%s/%s", msg, ns, apiVersion, kind, name), BodyDiff(diffContent))
}

// DebugBodyHead logs head of input slice.
func DebugBodyHead(logger *slog.Logger, msg, resourceType string, obj []byte) {
	limit := 1024
	switch resourceType {
	case "virtualmachines",
		"virtualmachines/status",
		"virtualmachineinstances",
		"virtualmachineinstances/status",
		"clustervirtualimages",
		"clustervirtualimages/status",
		"clusterrolebindings",
		"customresourcedefinitions":
		limit = 32000
	}
	if resourceType == "patch" {
		limit = len(obj)
	}
	logger.Debug(fmt.Sprintf("%s: dump rewritten body", msg), BodyDump(headBytes(obj, limit)))
}

func headBytes(msg []byte, limit int) string {
	s := string(msg)
	msgLen := len(s)
	if msgLen == 0 {
		return "<empty>"
	}
	// Lower the limit if message is shorter than the limit.
	if msgLen < limit {
		limit = msgLen
	}
	return fmt.Sprintf("[%d] %s", msgLen, s[0:limit])
}

// Diff returns a human-readable diff between 2 JSONs suitable for debugging.
// See: https://github.com/josephburnett/jd/blob/master/README.md
func Diff(json1, json2 []byte) (string, error) {
	// Handle some edge cases.
	switch {
	case json1 == nil && json2 != nil:
		return "", fmt.Errorf("got %d rewritten bytes without original", len(json2))
	case json1 != nil && json2 == nil:
		return "<No rewrite was done>", nil
	case json1 == nil && json2 == nil:
		return "<Empty>", nil
	case bytes.Equal(json1, json2):
		return "<Equal>", nil
	}

	// Calculate diff between JSONs.
	jd.Setkeys("name")
	a, err := jd.ReadJsonString(string(json1))
	if err != nil {
		return "", err
	}
	b, err := jd.ReadJsonString(string(json2))
	if err != nil {
		return "", err
	}
	return a.Diff(b).Render(), nil
}
