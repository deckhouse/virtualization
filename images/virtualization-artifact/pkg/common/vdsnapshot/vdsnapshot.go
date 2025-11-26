/*
Copyright 2025 Flant JSC

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

package vdsnapshot

import (
	"encoding/json"
	"fmt"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// AddOriginalMetadata adds original annotations and labels from VolumeSnapshot to VirtualDisk,
// without overwriting existing values
func AddOriginalMetadata(vd *v1alpha2.VirtualDisk, vs *vsv1.VolumeSnapshot) error {
	var (
		labelsMap      map[string]string
		annotationsMap map[string]string
	)

	if vs != nil && vs.Annotations != nil {
		if vs.Annotations[annotations.AnnVirtualDiskOriginalLabels] != "" {
			err := json.Unmarshal([]byte(vs.Annotations[annotations.AnnVirtualDiskOriginalLabels]), &labelsMap)
			if err != nil {
				return fmt.Errorf("failed to unmarshal the original labels: %w", err)
			}
		}

		if vs.Annotations[annotations.AnnVirtualDiskOriginalAnnotations] != "" {
			err := json.Unmarshal([]byte(vs.Annotations[annotations.AnnVirtualDiskOriginalAnnotations]), &annotationsMap)
			if err != nil {
				return fmt.Errorf("failed to unmarshal the original annotations: %w", err)
			}
		}
	}

	return nil
}
