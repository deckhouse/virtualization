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

package precheck

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	snapshotModuleCheckEnvName   = "SNAPSHOT_PRECHECK"
	deprecatedSnapshotModuleName = "snapshot-controller"
)

var snapshotRequiredModules = []string{"state-snapshotter", "storage-foundation"}

// snapshotPrecheck implements Precheck interface for the CSI snapshot machinery.
type snapshotPrecheck struct{}

func (s *snapshotPrecheck) Label() string {
	return PrecheckSnapshot
}

func (s *snapshotPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(snapshotModuleCheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("snapshot modules check is disabled.\n"))
		return nil
	}

	if err := RequireModuleDisabled(ctx, f, deprecatedSnapshotModuleName); err != nil {
		return fmt.Errorf("%s=no to disable this precheck: %w", snapshotModuleCheckEnvName, err)
	}

	for _, moduleName := range snapshotRequiredModules {
		if err := RequireModuleReady(ctx, f, moduleName); err != nil {
			return fmt.Errorf("%s=no to disable this precheck: %w", snapshotModuleCheckEnvName, err)
		}
	}

	// Check that at least one VolumeSnapshotClass exists
	gvr := schema.GroupVersionResource{
		Group:    "snapshot.storage.k8s.io",
		Version:  "v1",
		Resource: "volumesnapshotclasses",
	}

	vscList, err := f.DynamicClient().Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to list VolumeSnapshotClasses: %w", snapshotModuleCheckEnvName, err)
	}
	if len(vscList.Items) == 0 {
		return fmt.Errorf("%s=no to disable this precheck: no VolumeSnapshotClass found in the cluster", snapshotModuleCheckEnvName)
	}

	return nil
}

// Register Snapshot precheck (not common - requires explicit label).
func init() {
	RegisterPrecheck(&snapshotPrecheck{}, false)
}
