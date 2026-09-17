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

package vd

import (
	"context"
	"fmt"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// RestoreFloors are the two lower bounds a restore from a VirtualDiskSnapshot obeys. Keeping them apart
// is the point: asking for less than the source disk declared is a user error, while the driver floor is
// none of the user's doing and the target is grown to it silently.
//
// Either can be unset: a snapshot taken before the sizes were recorded carries neither.
type RestoreFloors struct {
	// Declared is what the source disk asked for, as recorded at capture time by the mechanism that took
	// the snapshot.
	Declared *resource.Quantity
	// Driver is the smallest volume the snapshot restores into. Rounding can push it above Declared.
	Driver *resource.Quantity

	vdSnapshotName string
}

// ResolveRestoreFloors reads the floors of vdSnapshot, fetching the CSI VolumeSnapshot when that is where
// they live. Callers holding it already pass it to RestoreFloorsFrom instead.
func ResolveRestoreFloors(ctx context.Context, c client.Client, vdSnapshot *v1alpha2.VirtualDiskSnapshot) (RestoreFloors, error) {
	if restorer.IsUnifiedDiskCapture(vdSnapshot) {
		return RestoreFloorsFrom(vdSnapshot, nil)
	}

	vs, err := object.FetchObject(ctx, types.NamespacedName{
		Name:      vdSnapshot.Status.VolumeSnapshotName,
		Namespace: vdSnapshot.Namespace,
	}, c, &vsv1.VolumeSnapshot{})
	if err != nil {
		return RestoreFloors{}, fmt.Errorf("fetch volume snapshot: %w", err)
	}

	return RestoreFloorsFrom(vdSnapshot, vs)
}

// RestoreFloorsFrom reads the floors whichever mechanism captured vdSnapshot recorded: a unified capture
// keeps both in its own status, the built-in one on the CSI VolumeSnapshot, which may be nil.
func RestoreFloorsFrom(vdSnapshot *v1alpha2.VirtualDiskSnapshot, vs *vsv1.VolumeSnapshot) (RestoreFloors, error) {
	floors := RestoreFloors{vdSnapshotName: vdSnapshot.Name}

	if restorer.IsUnifiedDiskCapture(vdSnapshot) {
		declared, err := parseSize(vdSnapshot.Status.PersistentVolumeClaimSize)
		if err != nil {
			return floors, fmt.Errorf("parse the captured PVC size: %w", err)
		}
		floors.Declared = declared

		if vdSnapshot.Status.Data != nil {
			driver, err := parseSize(vdSnapshot.Status.Data.Size)
			if err != nil {
				return floors, fmt.Errorf("parse the captured artifact size: %w", err)
			}
			floors.Driver = driver
		}

		return floors, nil
	}

	if vs == nil {
		return floors, nil
	}

	declared, err := parseSize(vs.Annotations[annotations.AnnVirtualDiskOriginalSize])
	if err != nil {
		return floors, fmt.Errorf("parse the original size: %w", err)
	}
	floors.Declared = declared

	if vs.Status != nil && vs.Status.RestoreSize != nil {
		driver := vs.Status.RestoreSize.DeepCopy()
		floors.Driver = &driver
	}

	return floors, nil
}

// Validate refuses a restore into less than the source disk declared.
func (f RestoreFloors) Validate(requested *resource.Quantity) error {
	if requested == nil || f.Declared == nil || requested.Cmp(*f.Declared) >= 0 {
		return nil
	}

	return service.NewInsufficientRestoreSizeError(f.vdSnapshotName, requested, f.Declared)
}

// Target is the size to provision: what was requested, grown to the driver floor. Without a request it
// falls back to the declared size, then to the driver floor — a disk provisioned from a snapshot carries
// an empty spec.persistentVolumeClaim, and an import records no declared size at all. Nil when the
// snapshot records neither floor.
func (f RestoreFloors) Target(requested *resource.Quantity) *resource.Quantity {
	size := requested
	if size == nil {
		size = f.Declared
	}
	if size == nil {
		return f.Driver
	}

	if f.Driver != nil && size.Cmp(*f.Driver) < 0 {
		return f.Driver
	}

	return size
}

func parseSize(raw string) (*resource.Quantity, error) {
	if raw == "" {
		return nil, nil
	}

	size, err := resource.ParseQuantity(raw)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", raw, err)
	}

	return &size, nil
}
