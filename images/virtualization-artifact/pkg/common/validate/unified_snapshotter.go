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

func UnifiedSnapshotterAnnotationAvailable(obj metav1.Object, present bool) error {
	if present {
		return nil
	}

	if _, ok := obj.GetAnnotations()[v1alpha2.AnnUseUnifiedSnapshotter]; !ok {
		return nil
	}

	return fmt.Errorf("the %s annotation requires the state-snapshotter module, which is not installed in this cluster", v1alpha2.AnnUseUnifiedSnapshotter)
}

func UnifiedSnapshotterAnnotationImmutable(oldObj, newObj metav1.Object) error {
	oldValue, oldOK := oldObj.GetAnnotations()[v1alpha2.AnnUseUnifiedSnapshotter]
	newValue, newOK := newObj.GetAnnotations()[v1alpha2.AnnUseUnifiedSnapshotter]

	if oldOK == newOK && oldValue == newValue {
		return nil
	}

	return fmt.Errorf("the %s annotation cannot be added, removed or changed: it selects the snapshot mechanism and is set once, at creation", v1alpha2.AnnUseUnifiedSnapshotter)
}
