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

package service

import (
	"context"
	"fmt"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

// EnsureVolumeSnapshotDeletable puts the delete-guard break-glass annotation on the
// VolumeSnapshot before the caller deletes it. The storage-foundation webhook marks
// every created VolumeSnapshot as an internal element of a unified snapshot
// (state-snapshotter.deckhouse.io/delete-protected), and the state-snapshotter
// delete guard then denies direct deletion to non-exempt actors, this controller
// included. A missing snapshot is not an error: the delete that follows tolerates
// it the same way.
func EnsureVolumeSnapshotDeletable(ctx context.Context, cl client.Client, vs *vsv1.VolumeSnapshot) error {
	if vs.Annotations[annotations.AnnAllowDelete] == "true" {
		return nil
	}

	base := vs.DeepCopy()
	annotations.AddAnnotation(vs, annotations.AnnAllowDelete, "true")
	err := cl.Patch(ctx, vs, client.MergeFrom(base))
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("annotate volume snapshot %s/%s as deletable: %w", vs.Namespace, vs.Name, err)
	}
	return nil
}
