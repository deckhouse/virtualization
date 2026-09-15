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

package validate

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// UnifiedSnapshotterAnnotationAvailable rejects pinning an object to the unified mechanism in a cluster
// where its controllers are not running: nothing would ever take that snapshot.
func UnifiedSnapshotterAnnotationAvailable(obj metav1.Object, present bool) error {
	if present {
		return nil
	}

	if _, ok := obj.GetAnnotations()[v1alpha2.AnnUseUnifiedSnapshotter]; !ok {
		return nil
	}

	return fmt.Errorf("the %s annotation requires the state-snapshotter module, which is not installed in this cluster", v1alpha2.AnnUseUnifiedSnapshotter)
}

// SnapshotterAnnotationsExclusive rejects an object that pins both mechanisms at once. Neither
// annotation is required — without them the snapshot follows the cluster default.
func SnapshotterAnnotationsExclusive(obj metav1.Object) error {
	annotations := obj.GetAnnotations()

	_, unified := annotations[v1alpha2.AnnUseUnifiedSnapshotter]
	_, builtIn := annotations[v1alpha2.AnnUseBuiltInSnapshotter]

	if !unified || !builtIn {
		return nil
	}

	return fmt.Errorf(
		"the %s and %s annotations are mutually exclusive: both select the snapshot mechanism, so set at most one",
		v1alpha2.AnnUseUnifiedSnapshotter, v1alpha2.AnnUseBuiltInSnapshotter,
	)
}

// SnapshotterAnnotationsImmutable keeps both mechanism-selecting annotations fixed for the life of the
// object. The mechanism decides how the snapshot is captured and, through the object's own status, how
// it is later read back — changing the request after the fact would only misdescribe what happened.
func SnapshotterAnnotationsImmutable(oldObj, newObj metav1.Object) error {
	for _, annotation := range []string{v1alpha2.AnnUseUnifiedSnapshotter, v1alpha2.AnnUseBuiltInSnapshotter} {
		oldValue, oldOK := oldObj.GetAnnotations()[annotation]
		newValue, newOK := newObj.GetAnnotations()[annotation]

		if oldOK != newOK || oldValue != newValue {
			return fmt.Errorf("the %s annotation cannot be added, removed or changed: it selects the snapshot mechanism and is set once, at creation", annotation)
		}
	}

	return nil
}

// ImportModeRequiresUnifiedSnapshotter rejects an import that nothing would ever materialize.
//
// spec.mode: Import means the snapshot's content arrives through the manifests-and-children-refs-upload
// subresource and is assembled by the state-snapshotter core. The built-in Secret-based mechanism has no
// part in that: pinned to it, or in a cluster where the core is not installed at all, an import-mode
// object would sit there while `d8 snapshot restore` waited on a bind that never comes. Refusing it at
// admission turns that silent wait into an answer at the moment the object is created.
func ImportModeRequiresUnifiedSnapshotter(obj metav1.Object, mode v1alpha2.UnifiedSnapshotterMode, present bool) error {
	if mode != v1alpha2.UnifiedSnapshotterModeImport {
		return nil
	}

	if _, builtIn := obj.GetAnnotations()[v1alpha2.AnnUseBuiltInSnapshotter]; builtIn {
		return fmt.Errorf(
			"spec.mode: %s cannot be combined with the %s annotation: an import is assembled by the state-snapshotter module, which the built-in mechanism has no part in",
			v1alpha2.UnifiedSnapshotterModeImport, v1alpha2.AnnUseBuiltInSnapshotter,
		)
	}

	if !present {
		return fmt.Errorf(
			"spec.mode: %s requires the state-snapshotter module, which is not installed in this cluster",
			v1alpha2.UnifiedSnapshotterModeImport,
		)
	}

	return nil
}
