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

// Package annotation gates this controller's reconcilers to only the objects the built-in
// virtualization-artifact controller has deliberately handed off, so the two controllers never fight
// over the same object.
package annotation

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// HasUnifiedSnapshotterAnnotation is a controller-runtime predicate matching only objects annotated with
// v1alpha2.AnnUseUnifiedSnapshotter. The built-in controller carries the mirror-image guard (it skips
// annotated objects), so an object is driven by exactly one of the two controllers at any time.
func HasUnifiedSnapshotterAnnotation() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		_, ok := obj.GetAnnotations()[v1alpha2.AnnUseUnifiedSnapshotter]
		return ok
	})
}
