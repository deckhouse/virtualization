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

// Package annotation gates this controller's reconcilers to only the objects the unified mechanism
// owns, so the two snapshot controllers never fight over the same object.
package annotation

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/deckhouse/virtualization-controller/pkg/common/snapshotter"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// unifiedIsDefault records that these controllers are registered only where
// UNIFIED_SNAPSHOTTER_PRESENT is set, so within this package the cluster default is always the unified
// mechanism. This is the one place that fact is spelled out.
const unifiedIsDefault = true

// DrivenByUnifiedVirtualMachineSnapshot and DrivenByUnifiedVirtualDiskSnapshot report whether the
// unified-snapshotter controllers own the object. They are the authoritative guards: the built-in
// controllers carry the mirror image over the same snapshotter routing, so an object is driven by
// exactly one of the two at any time.
func DrivenByUnifiedVirtualMachineSnapshot(vms *v1alpha2.VirtualMachineSnapshot) bool {
	return snapshotter.UseUnifiedForVirtualMachineSnapshot(vms, unifiedIsDefault)
}

// DrivenByUnifiedVirtualDiskSnapshot needs an uncached reader: it may have to read the owning
// VirtualMachineSnapshot's status, which a cache can still show without the captureState its controller
// wrote just before creating this child.
func DrivenByUnifiedVirtualDiskSnapshot(ctx context.Context, reader client.Reader, vds *v1alpha2.VirtualDiskSnapshot) (bool, error) {
	return snapshotter.UseUnifiedForVirtualDiskSnapshot(ctx, reader, vds, unifiedIsDefault)
}

// ShouldHandle is the manager-level event filter. It only has object metadata to go on, so it is a
// coarse filter: it drops objects pinned to the built-in mechanism and lets the rest reach a reconciler,
// whose typed guard above is what actually decides. Erring towards letting an object through is the
// safe direction — a filtered-out object would never be reconsidered.
func ShouldHandle() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return snapshotter.UseUnified(obj, unifiedIsDefault)
	})
}
