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

package rest

import (
	"encoding/json"
	"errors"
	"net/http"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/nodeapi"
	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/restore"
	"github.com/deckhouse/virtualization/api/subresources"
)

// apiError maps an error from the snapshot subresource services onto the Status the aggregated apiserver
// should answer with, shared by every subresource so one condition gets one answer everywhere.
//
// A StatusError carries a code and a reason our own service chose, and those are the answer: the client
// keys on them (ImportContentNotBound in particular is a retry signal, not a failure). A StatusError from
// a service is already a considered response, so it is never re-derived here.
func apiError(resource, name string, err error) error {
	var statusErr *k8serrors.StatusError
	var serviceErr *nodeapi.StatusError

	switch {
	case errors.As(err, &serviceErr):
		return statusErrorFrom(serviceErr)
	case errors.Is(err, restore.ErrSnapshotNotReady):
		return k8serrors.NewConflict(subresources.Resource(resource), name, err)
	case errors.As(err, &statusErr):
		return statusErr
	default:
		return k8serrors.NewInternalError(err)
	}
}

// statusErrorFrom rebuilds a *k8serrors.StatusError with our service's own code, reason and message, so
// the aggregated apiserver relays them unaltered instead of substituting a canonical reason for the code.
func statusErrorFrom(err *nodeapi.StatusError) *k8serrors.StatusError {
	return &k8serrors.StatusError{ErrStatus: statusFrom(err)}
}

// statusFrom renders a service rejection as a Kubernetes Status.
func statusFrom(err *nodeapi.StatusError) metav1.Status {
	return metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
		Status:   metav1.StatusFailure,
		Code:     int32(err.Code),
		Reason:   metav1.StatusReason(err.Reason),
		Message:  err.Message,
	}
}

// writeStatus writes a service rejection straight onto the wire.
//
// The upload subresource answers from inside its own handler rather than by returning an error, because
// only there can it keep a reason the aggregated apiserver has no mapping for: returning 409 through the
// error path would relabel ImportContentNotBound as a plain Conflict, and `d8 snapshot` distinguishes
// "wait for the binder" from "refused" by exactly that string.
func writeStatus(w http.ResponseWriter, err *nodeapi.StatusError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Code)
	_ = json.NewEncoder(w).Encode(statusFrom(err))
}
