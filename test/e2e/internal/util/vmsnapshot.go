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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

// UntilVMSnapshotsReady waits until every VirtualMachineSnapshot becomes Ready.
//
// TODO: will be refactored to observers.
//
// A snapshot that turns Failed because the CSI driver could not create an
// underlying VolumeSnapshot (the failure message relays a VolumeSnapshot error)
// skips the spec: that is a storage-infrastructure problem, not a
// virtualization one. Any other Failed reason fails the spec immediately
// instead of burning the whole timeout.
func UntilVMSnapshotsReady(ctx context.Context, f *framework.Framework, timeout time.Duration, snapshots ...*v1alpha2.VirtualMachineSnapshot) {
	GinkgoHelper()

	deadline := time.Now().Add(timeout)
	for {
		allReady := true
		for _, snapshot := range snapshots {
			err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(snapshot), snapshot)
			Expect(err).NotTo(HaveOccurred())

			switch snapshot.Status.Phase {
			case v1alpha2.VirtualMachineSnapshotPhaseReady:
			case v1alpha2.VirtualMachineSnapshotPhaseFailed:
				message := readyConditionMessage(snapshot.Status.Conditions, string(vmscondition.VirtualMachineSnapshotReadyType))
				if isCSIVolumeSnapshotError(message) {
					Skip(fmt.Sprintf(
						"VirtualMachineSnapshot %s/%s failed on the CSI side, skipping: %s",
						snapshot.Namespace, snapshot.Name, message,
					))
				}
				Fail(fmt.Sprintf(
					"VirtualMachineSnapshot %s/%s failed: %s",
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
			Fail(fmt.Sprintf("timed out after %s waiting for VirtualMachineSnapshots to be Ready: %s", timeout, strings.Join(names, ", ")))
		}

		time.Sleep(2 * time.Second)
	}
}
