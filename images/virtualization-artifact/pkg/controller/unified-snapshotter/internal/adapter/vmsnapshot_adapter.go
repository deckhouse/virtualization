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

// Package adapter implements github.com/deckhouse/state-snapshotter/pkg/snapshotsdk.SnapshotAdapter for
// VirtualMachineSnapshot and VirtualDiskSnapshot: the single seam mapping the SDK's generic capture
// protocol onto the existing (annotation-gated, additively-extended) status fields of those CRDs. Methods
// are side-effect free, as the SnapshotAdapter contract requires — the SDK owns all reads/writes/patching.
package adapter

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/deckhouse/state-snapshotter/api/storage/v1alpha1"
	"github.com/deckhouse/state-snapshotter/pkg/snapshotsdk"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// VirtualMachineSnapshotAdapter adapts a VirtualMachineSnapshot to snapshotsdk.SnapshotAdapter. It is a
// manifest-only aggregator node: no volume-capture leg of its own, children are the VM's VirtualDiskSnapshots.
type VirtualMachineSnapshotAdapter struct {
	VMS *v1alpha2.VirtualMachineSnapshot
}

var _ snapshotsdk.SnapshotAdapter = (*VirtualMachineSnapshotAdapter)(nil)

func (a *VirtualMachineSnapshotAdapter) Object() client.Object { return a.VMS }

// SourceRef identifies the live VirtualMachine this snapshot captures (spec.virtualMachineName).
func (a *VirtualMachineSnapshotAdapter) SourceRef() snapshotsdk.SourceRef {
	return snapshotsdk.SourceRef{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.VirtualMachineKind,
		Name:       a.VMS.Spec.VirtualMachineName,
	}
}

func (a *VirtualMachineSnapshotAdapter) GetDomainCaptureState() snapshotsdk.DomainCaptureState {
	return domainCaptureStateFromStatus(a.VMS.Status.CaptureState, a.VMS.Status.ChildrenSnapshotRefs)
}

func (a *VirtualMachineSnapshotAdapter) SetDomainCaptureState(st snapshotsdk.DomainCaptureState) {
	a.VMS.Status.CaptureState, a.VMS.Status.ChildrenSnapshotRefs = domainCaptureStateToStatus(a.VMS.Status.CaptureState, st)
}

func (a *VirtualMachineSnapshotAdapter) GetSnapshotSource() *snapshotsdk.SnapshotSource {
	return snapshotSourceFromRef(a.VMS.Status.SourceRef)
}

func (a *VirtualMachineSnapshotAdapter) SetSnapshotSource(src *snapshotsdk.SnapshotSource) {
	a.VMS.Status.SourceRef = snapshotSourceToRef(src)
}

func (a *VirtualMachineSnapshotAdapter) CoreCaptureState() snapshotsdk.CoreCaptureState {
	return coreCaptureStateFromStatus(a.VMS.Status.CaptureState)
}

func (a *VirtualMachineSnapshotAdapter) ReadyStatus() metav1.ConditionStatus {
	return readyStatus(a.VMS.Status.Conditions)
}

func (a *VirtualMachineSnapshotAdapter) ReadyReason() string {
	return readyReason(a.VMS.Status.Conditions)
}

func (a *VirtualMachineSnapshotAdapter) ReadyMessage() string {
	return readyMessage(a.VMS.Status.Conditions)
}

// --- shared helpers reused by VirtualDiskSnapshotAdapter ---

func domainCaptureStateFromStatus(cs *v1alpha2.UnifiedSnapshotterCaptureState, childRefs []v1alpha2.UnifiedSnapshotterChildRef) snapshotsdk.DomainCaptureState {
	out := snapshotsdk.DomainCaptureState{
		ChildrenSnapshotRefs: childRefsToSDK(childRefs),
		ExcludedRefs:         []snapshotsdk.ExcludedObjectRef{},
	}
	if cs == nil || cs.DomainSpecificController == nil {
		return out
	}
	d := cs.DomainSpecificController
	out.ManifestCaptureRequestName = d.ManifestCaptureRequestName
	out.VolumeCaptureRequestName = d.VolumeCaptureRequestName
	out.Phase = snapshotsdk.Phase(d.Phase)
	out.Reason = d.Reason
	out.Message = d.Message
	out.ExcludedRefs = excludedRefsToSDK(d.ExcludedRefs)
	return out
}

// domainCaptureStateToStatus merges an SDK-provided DomainCaptureState back into the status sub-structure,
// preserving the existing (core-owned) CommonController half untouched.
func domainCaptureStateToStatus(cur *v1alpha2.UnifiedSnapshotterCaptureState, st snapshotsdk.DomainCaptureState) (*v1alpha2.UnifiedSnapshotterCaptureState, []v1alpha2.UnifiedSnapshotterChildRef) {
	out := cur
	if out == nil {
		out = &v1alpha2.UnifiedSnapshotterCaptureState{}
	}
	out.DomainSpecificController = &v1alpha2.UnifiedSnapshotterDomainCaptureState{
		ManifestCaptureRequestName: st.ManifestCaptureRequestName,
		VolumeCaptureRequestName:   st.VolumeCaptureRequestName,
		Phase:                      v1alpha2.UnifiedSnapshotterPhase(st.Phase),
		Reason:                     st.Reason,
		Message:                    st.Message,
		ExcludedRefs:               excludedRefsFromSDK(st.ExcludedRefs),
	}
	return out, childRefsFromSDK(st.ChildrenSnapshotRefs)
}

func coreCaptureStateFromStatus(cs *v1alpha2.UnifiedSnapshotterCaptureState) snapshotsdk.CoreCaptureState {
	if cs == nil || cs.CommonController == nil {
		return snapshotsdk.CoreCaptureState{}
	}
	return snapshotsdk.CoreCaptureState{
		ManifestCaptured:                cs.CommonController.ManifestCaptured,
		DataCaptured:                    cs.CommonController.DataCaptured,
		ChildrenSettled:                 cs.CommonController.ChildrenSettled,
		ChildSubtreesManifestsPersisted: cs.CommonController.ChildSubtreesManifestsPersisted,
	}
}

func snapshotSourceFromRef(ref *v1alpha2.UnifiedSnapshotterSourceRef) *snapshotsdk.SnapshotSource {
	if ref == nil {
		return nil
	}
	src := snapshotsdk.SnapshotSource{
		APIVersion: ref.APIVersion,
		Kind:       ref.Kind,
		Name:       ref.Name,
		Namespace:  ref.Namespace,
		UID:        types.UID(ref.UID),
	}
	return &src
}

func snapshotSourceToRef(src *snapshotsdk.SnapshotSource) *v1alpha2.UnifiedSnapshotterSourceRef {
	if src == nil {
		return nil
	}
	return &v1alpha2.UnifiedSnapshotterSourceRef{
		APIVersion: src.APIVersion,
		Kind:       src.Kind,
		Name:       src.Name,
		Namespace:  src.Namespace,
		UID:        string(src.UID),
	}
}

func readyStatus(conditions []metav1.Condition) metav1.ConditionStatus {
	if c := meta.FindStatusCondition(conditions, v1alpha2.UnifiedSnapshotterConditionReady); c != nil {
		return c.Status
	}
	return ""
}

func readyReason(conditions []metav1.Condition) string {
	if c := meta.FindStatusCondition(conditions, v1alpha2.UnifiedSnapshotterConditionReady); c != nil {
		return c.Reason
	}
	return ""
}

func readyMessage(conditions []metav1.Condition) string {
	if c := meta.FindStatusCondition(conditions, v1alpha2.UnifiedSnapshotterConditionReady); c != nil {
		return c.Message
	}
	return ""
}

func childRefsToSDK(refs []v1alpha2.UnifiedSnapshotterChildRef) []storagev1alpha1.SnapshotChildRef {
	if refs == nil {
		return nil
	}
	out := make([]storagev1alpha1.SnapshotChildRef, len(refs))
	for i, r := range refs {
		out[i] = storagev1alpha1.SnapshotChildRef{APIVersion: r.APIVersion, Kind: r.Kind, Name: r.Name}
	}
	return out
}

func childRefsFromSDK(refs []storagev1alpha1.SnapshotChildRef) []v1alpha2.UnifiedSnapshotterChildRef {
	if refs == nil {
		return nil
	}
	out := make([]v1alpha2.UnifiedSnapshotterChildRef, len(refs))
	for i, r := range refs {
		out[i] = v1alpha2.UnifiedSnapshotterChildRef{APIVersion: r.APIVersion, Kind: r.Kind, Name: r.Name}
	}
	return out
}

func excludedRefsToSDK(refs []v1alpha2.UnifiedSnapshotterChildRef) []snapshotsdk.ExcludedObjectRef {
	out := make([]snapshotsdk.ExcludedObjectRef, len(refs))
	for i, r := range refs {
		out[i] = storagev1alpha1.ExcludedObjectRef{APIVersion: r.APIVersion, Kind: r.Kind, Name: r.Name}
	}
	return out
}

func excludedRefsFromSDK(refs []snapshotsdk.ExcludedObjectRef) []v1alpha2.UnifiedSnapshotterChildRef {
	out := make([]v1alpha2.UnifiedSnapshotterChildRef, len(refs))
	for i, r := range refs {
		out[i] = v1alpha2.UnifiedSnapshotterChildRef{APIVersion: r.APIVersion, Kind: r.Kind, Name: r.Name}
	}
	return out
}
