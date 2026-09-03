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

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"

	storagev1alpha1 "github.com/deckhouse/virtualization-controller/pkg/apis/storage/v1alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/volumemode"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
)

const (
	rwoImmediateStorageClassPrecheckEnvName = "RWO_IMMEDIATE_STORAGE_CLASS_PRECHECK"
)

// rwoImmediateStorageClassPrecheck fails the suite early when the StorageClass the
// tests will use (STORAGE_CLASS_NAME or the cluster default) is ReadWriteOnce with
// Immediate volume binding.
//
// Why this combination is rejected: with Immediate binding the CSI provisioner picks
// a node for every RWO volume at CreateVolume time — before any pod exists — so the
// choice ignores node taints and the placement of the workload's other volumes. The
// VirtualDisk importer needs its prime PVC, its scratch PVC and the pvc-importer pod
// together on one schedulable node. With RWO+Immediate the PVs can get pinned to a
// node the pod can never run on (e.g. a tainted control-plane) or to two different
// nodes; the pod then stays Pending forever and every VirtualDisk hangs in
// Provisioning. WaitForFirstConsumer has none of these failure modes: the scheduler
// picks the node for the actual pod and all its volumes bind there.
type rwoImmediateStorageClassPrecheck struct{}

func (c *rwoImmediateStorageClassPrecheck) Label() string {
	return PrecheckRWOImmediateStorageClass
}

func (c *rwoImmediateStorageClassPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(rwoImmediateStorageClassPrecheckEnvName) {
		return nil
	}

	k8sClient := f.GenericClient()
	var scList storagev1.StorageClassList
	if err := k8sClient.List(ctx, &scList); err != nil {
		return fmt.Errorf("%s=no to disable this precheck: list StorageClasses: %w", rwoImmediateStorageClassPrecheckEnvName, err)
	}

	sc, err := config.ResolveDefaultStorageClass(&scList)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: %w", rwoImmediateStorageClassPrecheckEnvName, err)
	}
	if sc == nil {
		// No suite StorageClass at all: defaultStorageClassPrecheck reports that.
		return nil
	}

	if sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
		return nil
	}

	// Resolve the access mode the same way the virtualization-controller does:
	// annotations on the StorageClass first, then the StorageProfile capabilities.
	modeGetter := volumemode.NewVolumeAndAccessModesGetter(k8sClient, func(ctx context.Context, name string) (*storagev1alpha1.StorageProfile, error) {
		obj := &rewrite.StorageProfile{}
		if err := f.RewriteClient().Get(ctx, name, obj); err != nil {
			return nil, err
		}
		return obj.StorageProfile, nil
	})
	_, accessMode, err := modeGetter.GetVolumeAndAccessModes(ctx, sc, sc)
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: resolve access mode for StorageClass %q: %w", rwoImmediateStorageClassPrecheckEnvName, sc.Name, err)
	}

	if accessMode == corev1.ReadWriteMany {
		return nil
	}

	return fmt.Errorf("%s=no to disable this precheck: StorageClass %q is %s with Immediate volume binding and must not be used for e2e tests: "+
		"with Immediate binding the CSI provisioner picks a node for every RWO volume before any pod exists, ignoring node taints and the placement "+
		"of the workload's other volumes, so the VirtualDisk importer's prime/scratch PVCs can get pinned to a node the pvc-importer pod can never "+
		"run on (or to two different nodes) — the pod stays Pending forever and disks hang in Provisioning. "+
		"Use a WaitForFirstConsumer StorageClass instead (set %s)",
		rwoImmediateStorageClassPrecheckEnvName, sc.Name, accessMode, config.StorageClassNameEnv)
}

// Register rwoImmediateStorageClassPrecheck as common (runs for all tests).
func init() {
	RegisterPrecheck(&rwoImmediateStorageClassPrecheck{}, true)
}
