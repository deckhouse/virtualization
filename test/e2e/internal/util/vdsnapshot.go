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

package util

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

// UntilVDSnapshotsReady waits until every VirtualDiskSnapshot becomes Ready.
//
// TODO: will be refactored to observers.
//
// A snapshot that turns Failed because the CSI driver could not create the
// underlying VolumeSnapshot (the failure message relays a VolumeSnapshot error)
// skips the spec: that is a storage-infrastructure problem, not a
// virtualization one. Any other Failed reason fails the spec immediately
// instead of burning the whole timeout.
func UntilVDSnapshotsReady(ctx context.Context, f *framework.Framework, timeout time.Duration, snapshots ...*v1alpha2.VirtualDiskSnapshot) {
	GinkgoHelper()

	deadline := time.Now().Add(timeout)
	for {
		allReady := true
		for _, snapshot := range snapshots {
			err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(snapshot), snapshot)
			Expect(err).NotTo(HaveOccurred())

			switch snapshot.Status.Phase {
			case v1alpha2.VirtualDiskSnapshotPhaseReady:
			case v1alpha2.VirtualDiskSnapshotPhaseFailed:
				message := vdSnapshotReadyConditionMessage(snapshot)
				if isCSIVolumeSnapshotError(message) {
					Skip(fmt.Sprintf(
						"VirtualDiskSnapshot %s/%s failed on the CSI side, skipping: %s",
						snapshot.Namespace, snapshot.Name, message,
					))
				}
				Fail(fmt.Sprintf(
					"VirtualDiskSnapshot %s/%s failed: %s",
					snapshot.Namespace, snapshot.Name, message,
				))
			default:
				allReady = false
			}
		}

		if allReady {
			return
		}
		if time.Now().After(deadline) {
			names := make([]string, 0, len(snapshots))
			for _, snapshot := range snapshots {
				names = append(names, fmt.Sprintf("%s=%s", snapshot.Name, snapshot.Status.Phase))
			}
			Fail(fmt.Sprintf("timed out after %s waiting for VirtualDiskSnapshots to be Ready: %s", timeout, strings.Join(names, ", ")))
		}

		time.Sleep(2 * time.Second)
	}
}

// vdSnapshotReadyConditionMessage returns the message of the snapshot's Ready
// condition, or an empty string when the condition is not present.
func vdSnapshotReadyConditionMessage(snapshot *v1alpha2.VirtualDiskSnapshot) string {
	for _, cond := range snapshot.Status.Conditions {
		if cond.Type == "Ready" {
			return cond.Message
		}
	}
	return ""
}

// isCSIVolumeSnapshotError reports whether the failure message relays an error
// of the underlying VolumeSnapshot, i.e. the snapshot was accepted by the
// virtualization controller but the CSI driver failed to take it (see the
// "VolumeSnapshot %q has an error: ..." message in the vdsnapshot lifecycle).
func isCSIVolumeSnapshotError(message string) bool {
	return strings.Contains(message, "VolumeSnapshot") && strings.Contains(message, "has an error")
}
