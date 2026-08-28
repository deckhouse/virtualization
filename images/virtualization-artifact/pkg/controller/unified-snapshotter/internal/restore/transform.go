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

package restore

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk/transform"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// Transformer is our github.com/deckhouse/state-snapshotter/pkg/snapshotsdk/transform.Transformer
// implementation: it points a restored VirtualDisk at its own VirtualDiskSnapshot via
// spec.dataSource.objectRef, the same field shape the existing (unmodified) VirtualDisk controller
// already knows how to provision from — see vdsnapshot.ensureVolumeSnapshotBridge for the other half of
// that contract (VirtualDiskSnapshot.status.volumeSnapshotName).
type Transformer struct{}

var _ transform.Transformer = Transformer{}

// CoveredPVCNames is always empty: our manifest captures are VirtualMachine/VirtualDisk objects only,
// never PersistentVolumeClaims directly.
func (Transformer) CoveredPVCNames(_ *transform.RestoreNode, _ []unstructured.Unstructured) map[string]struct{} {
	return map[string]struct{}{}
}

// TransformObject sets a VirtualDisk's spec.dataSource to its own owning VirtualDiskSnapshot node.
func (Transformer) TransformObject(node *transform.RestoreNode, obj *unstructured.Unstructured, _ []transform.NodeResult) (bool, error) {
	if !isVirtualDisk(*obj) {
		return false, nil
	}
	if node == nil || node.SnapshotRef.Kind != v1alpha2.VirtualDiskSnapshotKind {
		return false, nil
	}
	dataSource := map[string]interface{}{
		"type": string(v1alpha2.DataSourceTypeObjectRef),
		"objectRef": map[string]interface{}{
			"kind": string(v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot),
			"name": node.SnapshotRef.Name,
		},
	}
	if err := unstructured.SetNestedMap(obj.Object, dataSource, "spec", "dataSource"); err != nil {
		return false, fmt.Errorf("set VirtualDisk %s spec.dataSource: %w", obj.GetName(), err)
	}
	return true, nil
}

func isVirtualDisk(obj unstructured.Unstructured) bool {
	return obj.GetKind() == v1alpha2.VirtualDiskKind && obj.GetAPIVersion() == v1alpha2.SchemeGroupVersion.String()
}
