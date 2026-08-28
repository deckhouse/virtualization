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

package adapter

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// VirtualDiskSnapshotAdapter adapts a VirtualDiskSnapshot to snapshotsdk.SnapshotAdapter. It is a
// data-bearing leaf node: no children of its own, its volume-capture leg is the disk's backing PVC.
type VirtualDiskSnapshotAdapter struct {
	VDS *v1alpha2.VirtualDiskSnapshot
}

var _ snapshotsdk.SnapshotAdapter = (*VirtualDiskSnapshotAdapter)(nil)

func (a *VirtualDiskSnapshotAdapter) Object() client.Object { return a.VDS }

// SourceRef identifies the live VirtualDisk this snapshot captures (spec.virtualDiskName).
func (a *VirtualDiskSnapshotAdapter) SourceRef() snapshotsdk.SourceRef {
	return snapshotsdk.SourceRef{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.VirtualDiskKind,
		Name:       a.VDS.Spec.VirtualDiskName,
	}
}

func (a *VirtualDiskSnapshotAdapter) GetDomainCaptureState() snapshotsdk.DomainCaptureState {
	return domainCaptureStateFromStatus(a.VDS.Status.CaptureState, a.VDS.Status.ChildrenSnapshotRefs)
}

func (a *VirtualDiskSnapshotAdapter) SetDomainCaptureState(st snapshotsdk.DomainCaptureState) {
	a.VDS.Status.CaptureState, a.VDS.Status.ChildrenSnapshotRefs = domainCaptureStateToStatus(a.VDS.Status.CaptureState, st)
}

func (a *VirtualDiskSnapshotAdapter) GetSnapshotSource() *snapshotsdk.SnapshotSource {
	return snapshotSourceFromRef(a.VDS.Status.SourceRef)
}

func (a *VirtualDiskSnapshotAdapter) SetSnapshotSource(src *snapshotsdk.SnapshotSource) {
	a.VDS.Status.SourceRef = snapshotSourceToRef(src)
}

func (a *VirtualDiskSnapshotAdapter) CoreCaptureState() snapshotsdk.CoreCaptureState {
	return coreCaptureStateFromStatus(a.VDS.Status.CaptureState)
}

func (a *VirtualDiskSnapshotAdapter) ReadyStatus() metav1.ConditionStatus {
	return readyStatus(a.VDS.Status.Conditions)
}

func (a *VirtualDiskSnapshotAdapter) ReadyReason() string {
	return readyReason(a.VDS.Status.Conditions)
}

func (a *VirtualDiskSnapshotAdapter) ReadyMessage() string {
	return readyMessage(a.VDS.Status.Conditions)
}
