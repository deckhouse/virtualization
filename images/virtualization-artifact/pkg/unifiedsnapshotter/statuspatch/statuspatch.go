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

// Package statuspatch builds JSON merge patches for CRD status subresource writes that are safe against
// the apiserver's "Object 'Kind' is missing" rejection, and against clobbering fields another controller
// (the state-snapshotter core, the SDK) concurrently writes to the same object.
//
// Earlier versions of this package computed a merge patch by diffing json.Marshal(base) against
// json.Marshal(current) (mirroring client.MergeFrom), then tried to force apiVersion/kind into the
// result. That never reliably avoided the apiserver's rejection in practice — see git history for the
// failed attempts. Diffing was also solving a problem the caller doesn't have: every call site already
// knows exactly which status fields it owns and wants to write; it doesn't need to be discovered by
// comparing two full-object snapshots. So this package doesn't diff anything — the caller passes the
// desired status value (a small struct or map covering only the fields it owns), and this package just
// wraps it with apiVersion/kind and marshals it. Only fields the caller lists are ever touched, so this
// is also naturally safe against clobbering fields owned by the core/SDK, which the caller never mentions.
package statuspatch

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// For builds a merge patch of the form {"apiVersion": ..., "kind": ..., "status": status}. status should
// be a struct or map covering only the fields the caller owns and wants to set — every field it carries
// (that marshals to a non-omitted value) overwrites the stored value; fields it doesn't mention are left
// untouched on the server, including ones owned by another writer.
func For(gvk schema.GroupVersionKind, status any) (client.Patch, error) {
	body := map[string]any{
		"apiVersion": gvk.GroupVersion().String(),
		"kind":       gvk.Kind,
		"status":     status,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal status patch: %w", err)
	}
	return client.RawPatch(types.MergePatchType, data), nil
}
