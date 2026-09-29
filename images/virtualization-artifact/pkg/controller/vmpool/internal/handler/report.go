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

package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// passReport collects what the handlers did to pool members during one reconcile
// pass. Failures of API calls are not visible on the objects afterwards, so this
// is the only way the status handler learns about them.
type passReport struct {
	scaleAttempted bool
	creation       string
	deletion       string
	update         string
	updateFailed   map[string]bool
}

type reportKey struct{}

// WithReport returns a context that carries a fresh report for one reconcile pass.
func WithReport(ctx context.Context) context.Context {
	return context.WithValue(ctx, reportKey{}, &passReport{})
}

// ReportFrom returns the report of the current pass. Without one (a handler run
// on its own, as in unit tests) it returns a report nobody reads.
func ReportFrom(ctx context.Context) *passReport {
	if r, ok := ctx.Value(reportKey{}).(*passReport); ok {
		return r
	}
	return &passReport{}
}

// ScaleAttempted marks that the sync handler compared the replica count with the
// desired one in this pass, rather than waiting for its previous actions to be observed.
func (r *passReport) ScaleAttempted() { r.scaleAttempted = true }

// The first failure of each kind wins: one cause is enough to act on, and a stable
// choice keeps the message from changing between passes.

func (r *passReport) CreationFailed(msg string) {
	if r.creation == "" {
		r.creation = msg
	}
}

func (r *passReport) DeletionFailed(msg string) {
	if r.deletion == "" {
		r.deletion = msg
	}
}

func (r *passReport) UpdateFailed(member, msg string) {
	if r.update == "" {
		r.update = msg
	}
	if r.updateFailed == nil {
		r.updateFailed = map[string]bool{}
	}
	r.updateFailed[member] = true
}

// isPersistent tells an error the user has to act on from one a retry fixes by
// itself (conflict, timeout, throttling, an unavailable webhook), which must not
// make the status flap.
func isPersistent(err error) bool {
	return apierrors.IsInvalid(err) || apierrors.IsForbidden(err) || apierrors.IsBadRequest(err)
}

// Field paths in API errors are relative to the object the pool creates; these
// prefixes turn them into paths in the pool spec, where the user edits them.
const vmTemplatePath = "spec.virtualMachineTemplate."

func diskTemplatePath(name string) string {
	return fmt.Sprintf("spec.virtualDiskTemplates[%s].", name)
}

// explain turns an API error into a sentence for the pool status: the cause and
// what to do, with the field path at the end. The raw error stays in the events.
func explain(err error, fieldPrefix string) string {
	var status apierrors.APIStatus
	if !errors.As(err, &status) {
		return "unexpected error."
	}
	st := status.Status()
	denial, denied := denialReason(st.Message)
	switch {
	case st.Reason == metav1.StatusReasonInvalid && st.Details != nil && len(st.Details.Causes) > 0 && st.Details.Causes[0].Field != "":
		cause := st.Details.Causes[0]
		return fmt.Sprintf("the template is rejected: %s (%s%s). Fix the template.", strings.TrimSuffix(cause.Message, "."), fieldPrefix, cause.Field)
	case st.Reason == metav1.StatusReasonForbidden && strings.Contains(st.Message, "exceeded quota"):
		return "the namespace ResourceQuota is exceeded. The pool retries automatically; contact your administrator to increase the quota."
	case denied:
		return fmt.Sprintf("the template is rejected: %s. Fix the template.", denial)
	case st.Reason == metav1.StatusReasonForbidden:
		return "access is denied. Contact your administrator."
	default:
		return fmt.Sprintf("the request is rejected: %s. Fix the template.", strings.TrimSuffix(st.Message, "."))
	}
}

// explainDeletion is explain for deleting a replica: the template has nothing to
// do with it, so the cause is not blamed on the template.
func explainDeletion(err error) string {
	var status apierrors.APIStatus
	if !errors.As(err, &status) {
		return "unexpected error."
	}
	st := status.Status()
	if denial, denied := denialReason(st.Message); denied {
		return fmt.Sprintf("the deletion is denied: %s.", denial)
	}
	if st.Reason == metav1.StatusReasonForbidden {
		return "access is denied. Contact your administrator."
	}
	return fmt.Sprintf("the request is rejected: %s.", strings.TrimSuffix(st.Message, "."))
}

// An admission webhook says "denied the request: ", a ValidatingAdmissionPolicy says
// "denied request: " and sets the reason it is configured with (Invalid by default).
// The text after the marker is the rejecting party's own reason.
var denialMarkers = []string{"denied the request: ", "denied request: "}

func denialReason(msg string) (string, bool) {
	for _, marker := range denialMarkers {
		if _, reason, ok := strings.Cut(msg, marker); ok {
			return strings.TrimSuffix(reason, "."), true
		}
	}
	return "", false
}
