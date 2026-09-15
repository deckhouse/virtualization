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

package restorer

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/node"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// ManifestReader is the read-side abstraction SnapshotResources.Prepare uses to reconstruct a
// VirtualMachine and its network/provisioning/hotplug companions from a captured VirtualMachineSnapshot.
// It has two implementations: secretManifestReader (the built-in mechanism's snapshot Secret) and
// unifiedManifestReader (the unified-snapshotter SDK's captured SnapshotContent). NewManifestReader picks
// between them on IsUnifiedCapture.
//
// VirtualMachine is mandatory (every capture mechanism always declares it); the rest are optional —
// implementations return (nil, nil), not an error, when the corresponding resource wasn't captured
// (mirroring the old mechanism's get[T] zero-value-on-missing-key behavior).
type ManifestReader interface {
	RestoreVirtualMachine(ctx context.Context) (*v1alpha2.VirtualMachine, error)
	RestoreProvisioner(ctx context.Context) (*corev1.Secret, error)
	RestoreVirtualMachineIPAddress(ctx context.Context) (*v1alpha2.VirtualMachineIPAddress, error)
	RestoreVirtualMachineMACAddresses(ctx context.Context) ([]*v1alpha2.VirtualMachineMACAddress, error)
	RestoreMACAddressOrder(ctx context.Context) ([]string, error)
	RestoreVirtualMachineBlockDeviceAttachments(ctx context.Context) ([]*v1alpha2.VirtualMachineBlockDeviceAttachment, error)
}

// IsUnifiedCapture reports whether vmSnapshot's content was produced by the state-snapshotter core
// rather than by the built-in Secret-based mechanism, and so which of the two ManifestReader
// implementations can read it back.
//
// status.captureState answers that for a capture, and spec.mode answers it for an import. The two are
// both needed and neither is redundant: an import is materialized from an uploaded payload and never
// enters the capture state machine, so captureState stays nil on it forever — read alone it would send
// every imported snapshot down the built-in path, looking for a snapshot Secret that no import creates.
func IsUnifiedCapture(vmSnapshot *v1alpha2.VirtualMachineSnapshot) bool {
	return vmSnapshot != nil && (vmSnapshot.Status.CaptureState != nil || vmSnapshot.IsImport())
}

// IsUnifiedDiskCapture is IsUnifiedCapture for a VirtualDiskSnapshot: same discriminator, same reason.
//
// A unified capture leaves no bound CSI VolumeSnapshot to restore from — the data is referenced by
// status.data.artifactRef instead — so every restore path that would reach for status.volumeSnapshotName
// has to branch on this first. That is exactly the path `d8 snapshot restore` puts an import on: the
// manifests it applies point each VirtualDisk at its own VirtualDiskSnapshot.
func IsUnifiedDiskCapture(vdSnapshot *v1alpha2.VirtualDiskSnapshot) bool {
	return vdSnapshot != nil && (vdSnapshot.Status.CaptureState != nil || vdSnapshot.IsImport())
}

// NewManifestReader resolves the right ManifestReader for vmSnapshot: the unified-snapshotter SDK's
// captured SnapshotContent when the SDK captured it (there is no snapshot Secret at all in that case),
// or the built-in mechanism's snapshot Secret otherwise.
func NewManifestReader(ctx context.Context, c client.Client, vmSnapshot *v1alpha2.VirtualMachineSnapshot) (ManifestReader, error) {
	if IsUnifiedCapture(vmSnapshot) {
		if vmSnapshot.Status.BoundSnapshotContentName == "" {
			// The state-snapshotter core hasn't bound a SnapshotContent to this VirtualMachineSnapshot
			// yet; retryable, not a real error.
			return nil, common.ErrQueueing
		}
		if err := verifySnapshotContentBackRef(ctx, c, vmSnapshot.Status.BoundSnapshotContentName, snapshotSubject{
			kind:      v1alpha2.VirtualMachineSnapshotKind,
			namespace: vmSnapshot.Namespace,
			name:      vmSnapshot.Name,
			uid:       vmSnapshot.UID,
		}); err != nil {
			return nil, err
		}
		cc, err := sharedContentClient()
		if err != nil {
			return nil, err
		}
		return &unifiedManifestReader{
			client:        cc,
			contentName:   vmSnapshot.Status.BoundSnapshotContentName,
			keepIPAddress: vmSnapshot.Spec.KeepIPAddress,
		}, nil
	}

	restorerSecretKey := types.NamespacedName{Namespace: vmSnapshot.Namespace, Name: vmSnapshot.Status.VirtualMachineSnapshotSecretName}
	secret, err := object.FetchObject(ctx, restorerSecretKey, c, &corev1.Secret{})
	if err != nil {
		return nil, err
	}
	if secret == nil {
		return nil, fmt.Errorf("restorer secret %q is not found", restorerSecretKey.Name)
	}
	return &secretManifestReader{restorer: NewSecretRestorer(c), secret: secret}, nil
}

type snapshotSubject struct {
	kind      string
	namespace string
	name      string
	uid       types.UID
}

func (s snapshotSubject) String() string {
	return fmt.Sprintf("%s %s/%s", s.kind, s.namespace, s.name)
}

// verifySnapshotContentBackRef is the anti-spoofing handshake for reading a SnapshotContent addressed by
// a status.boundSnapshotContentName: the content's spec.snapshotRef must point back at the very resource
// that named it.
//
// The rule itself lives in one place (node.ResolveBoundContent), shared with the three aggregated
// subresources that read and write the same contents. It must: a second implementation that compared one
// field fewer would be a way to read another namespace's captured manifests, and the two would drift
// apart the first time either was edited. Only the wording is this package's own, because these messages
// end up in a VirtualDisk or VirtualMachineOperation condition a user reads.
func verifySnapshotContentBackRef(ctx context.Context, c client.Client, contentName string, subject snapshotSubject) error {
	_, err := node.ResolveBoundContent(ctx, c, node.Node{
		APIVersion:  v1alpha2.SchemeGroupVersion.String(),
		Kind:        subject.kind,
		Namespace:   subject.namespace,
		Name:        subject.name,
		UID:         subject.uid,
		ContentName: contentName,
	})
	switch {
	case err == nil:
		return nil
	case apierrors.IsNotFound(err):
		return fmt.Errorf(
			"SnapshotContent %q named by %s status.boundSnapshotContentName does not exist: its captured manifests are gone and the snapshot can no longer be restored",
			contentName, subject,
		)
	case apierrors.IsForbidden(err):
		return fmt.Errorf("%s cannot restore from SnapshotContent %q: %w", subject, contentName, err)
	default:
		return fmt.Errorf("get SnapshotContent %q bound to %s: %w", contentName, subject, err)
	}
}

// CapturedVirtualDisk returns the VirtualDisk manifest captured under vdSnapshot's own bound
// SnapshotContent, or (nil, nil) when vdSnapshot was captured by the built-in mechanism and has no such
// content.
func CapturedVirtualDisk(ctx context.Context, c client.Client, vdSnapshot *v1alpha2.VirtualDiskSnapshot) (*v1alpha2.VirtualDisk, error) {
	if !IsUnifiedDiskCapture(vdSnapshot) {
		return nil, nil
	}
	if vdSnapshot.Status.BoundSnapshotContentName == "" {
		return nil, common.ErrQueueing
	}
	if err := verifySnapshotContentBackRef(ctx, c, vdSnapshot.Status.BoundSnapshotContentName, snapshotSubject{
		kind:      v1alpha2.VirtualDiskSnapshotKind,
		namespace: vdSnapshot.Namespace,
		name:      vdSnapshot.Name,
		uid:       vdSnapshot.UID,
	}); err != nil {
		return nil, err
	}

	cc, err := sharedContentClient()
	if err != nil {
		return nil, err
	}
	return capturedVirtualDiskFrom(ctx, cc, vdSnapshot.Status.BoundSnapshotContentName)
}

func capturedVirtualDiskFrom(ctx context.Context, downloader manifestDownloader, contentName string) (*v1alpha2.VirtualDisk, error) {
	manifests, err := downloader.DownloadManifests(ctx, contentName)
	if err != nil {
		return nil, err
	}
	for _, m := range manifests {
		if !m.Is(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualDiskKind) {
			continue
		}
		vd := &v1alpha2.VirtualDisk{}
		if err := json.Unmarshal(m.Raw, vd); err != nil {
			return nil, fmt.Errorf("unmarshal captured VirtualDisk from SnapshotContent %q: %w", contentName, err)
		}
		return vd, nil
	}
	return nil, fmt.Errorf("no VirtualDisk manifest found in SnapshotContent %q", contentName)
}

// secretManifestReader adapts the existing *SecretRestorer (unchanged) to ManifestReader by binding the
// secret it reads from once, instead of taking it per-call.
type secretManifestReader struct {
	restorer *SecretRestorer
	secret   *corev1.Secret
}

func (s *secretManifestReader) RestoreVirtualMachine(ctx context.Context) (*v1alpha2.VirtualMachine, error) {
	return s.restorer.RestoreVirtualMachine(ctx, s.secret)
}

func (s *secretManifestReader) RestoreProvisioner(ctx context.Context) (*corev1.Secret, error) {
	return s.restorer.RestoreProvisioner(ctx, s.secret)
}

func (s *secretManifestReader) RestoreVirtualMachineIPAddress(ctx context.Context) (*v1alpha2.VirtualMachineIPAddress, error) {
	return s.restorer.RestoreVirtualMachineIPAddress(ctx, s.secret)
}

func (s *secretManifestReader) RestoreVirtualMachineMACAddresses(ctx context.Context) ([]*v1alpha2.VirtualMachineMACAddress, error) {
	return s.restorer.RestoreVirtualMachineMACAddresses(ctx, s.secret)
}

func (s *secretManifestReader) RestoreMACAddressOrder(ctx context.Context) ([]string, error) {
	return s.restorer.RestoreMACAddressOrder(ctx, s.secret)
}

func (s *secretManifestReader) RestoreVirtualMachineBlockDeviceAttachments(ctx context.Context) ([]*v1alpha2.VirtualMachineBlockDeviceAttachment, error) {
	return s.restorer.RestoreVirtualMachineBlockDeviceAttachments(ctx, s.secret)
}

// manifestDownloader is the subset of *ContentClient unifiedManifestReader depends on, broken out so
// tests can substitute a fake instead of making a real aggregated-APIService call.
type manifestDownloader interface {
	DownloadManifests(ctx context.Context, snapshotContentName string) ([]RawManifest, error)
}

// unifiedManifestReader reads the same 5 logical resources back out of a unified-snapshotter
// SnapshotContent's manifests-download response, demuxed by apiVersion+kind — the resources are exactly the targets
// vmsnapshot's manifest_targets.go declared at capture time (VirtualMachine, VirtualMachineIPAddress,
// VirtualMachineMACAddress x N, one provisioner Secret, VirtualMachineBlockDeviceAttachment x N).
type unifiedManifestReader struct {
	client        manifestDownloader
	contentName   string
	keepIPAddress v1alpha2.KeepIPAddress

	once      sync.Once
	manifests []RawManifest
	loadErr   error
}

func (u *unifiedManifestReader) load(ctx context.Context) ([]RawManifest, error) {
	u.once.Do(func() {
		u.manifests, u.loadErr = u.client.DownloadManifests(ctx, u.contentName)
	})
	return u.manifests, u.loadErr
}

func (u *unifiedManifestReader) RestoreVirtualMachine(ctx context.Context) (*v1alpha2.VirtualMachine, error) {
	manifests, err := u.load(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range manifests {
		if !m.Is(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineKind) {
			continue
		}
		vm := &v1alpha2.VirtualMachine{}
		if err := json.Unmarshal(m.Raw, vm); err != nil {
			return nil, fmt.Errorf("unmarshal captured VirtualMachine from SnapshotContent %q: %w", u.contentName, err)
		}
		return vm, nil
	}
	return nil, fmt.Errorf("no VirtualMachine manifest found in SnapshotContent %q", u.contentName)
}

func (u *unifiedManifestReader) RestoreProvisioner(ctx context.Context) (*corev1.Secret, error) {
	vm, err := u.RestoreVirtualMachine(ctx)
	if err != nil {
		return nil, err
	}

	secretName := provisionerSecretName(vm)
	if secretName == "" {
		return nil, nil
	}

	manifests, err := u.load(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range manifests {
		if !m.Is("v1", "Secret") {
			continue
		}
		secret := &corev1.Secret{}
		if err := json.Unmarshal(m.Raw, secret); err != nil {
			return nil, fmt.Errorf("unmarshal captured provisioner Secret from SnapshotContent %q: %w", u.contentName, err)
		}
		if secret.Name != secretName {
			continue
		}
		return secret, nil
	}
	return nil, fmt.Errorf("provisioner Secret %q referenced by the captured VirtualMachine %q is missing from SnapshotContent %q", secretName, vm.Name, u.contentName)
}

func provisionerSecretName(vm *v1alpha2.VirtualMachine) string {
	p := vm.Spec.Provisioning
	if p == nil {
		return ""
	}

	switch p.Type {
	case v1alpha2.ProvisioningTypeUserDataRef:
		if p.UserDataRef == nil || p.UserDataRef.Kind != v1alpha2.UserDataRefKindSecret {
			return ""
		}
		return p.UserDataRef.Name
	case v1alpha2.ProvisioningTypeSysprepRef:
		if p.SysprepRef == nil || p.SysprepRef.Kind != v1alpha2.SysprepRefKindSecret {
			return ""
		}
		return p.SysprepRef.Name
	default:
		return ""
	}
}

func (u *unifiedManifestReader) RestoreVirtualMachineIPAddress(ctx context.Context) (*v1alpha2.VirtualMachineIPAddress, error) {
	manifests, err := u.load(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range manifests {
		if !m.Is(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineIPAddressKind) {
			continue
		}
		vmip := &v1alpha2.VirtualMachineIPAddress{}
		if err := json.Unmarshal(m.Raw, vmip); err != nil {
			return nil, fmt.Errorf("unmarshal captured VirtualMachineIPAddress from SnapshotContent %q: %w", u.contentName, err)
		}
		if err := u.applyKeepIPAddress(vmip); err != nil {
			return nil, err
		}
		return vmip, nil
	}
	return nil, nil
}

func (u *unifiedManifestReader) applyKeepIPAddress(vmip *v1alpha2.VirtualMachineIPAddress) error {
	if u.keepIPAddress != v1alpha2.KeepIPAddressAlways || vmip.Spec.Type != v1alpha2.VirtualMachineIPAddressTypeAuto {
		return nil
	}

	if vmip.Status.Address == "" {
		return fmt.Errorf(
			"cannot honor keepIPAddress=Always: captured VirtualMachineIPAddress %q from SnapshotContent %q has an Auto type and no allocated address",
			vmip.Name, u.contentName,
		)
	}

	vmip.Spec.Type = v1alpha2.VirtualMachineIPAddressTypeStatic
	vmip.Spec.StaticIP = vmip.Status.Address
	return nil
}

func (u *unifiedManifestReader) RestoreVirtualMachineMACAddresses(ctx context.Context) ([]*v1alpha2.VirtualMachineMACAddress, error) {
	manifests, err := u.load(ctx)
	if err != nil {
		return nil, err
	}
	var vmmacs []*v1alpha2.VirtualMachineMACAddress
	for _, m := range manifests {
		if !m.Is(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineMACAddressKind) {
			continue
		}
		vmmac := &v1alpha2.VirtualMachineMACAddress{}
		if err := json.Unmarshal(m.Raw, vmmac); err != nil {
			return nil, fmt.Errorf("unmarshal captured VirtualMachineMACAddress from SnapshotContent %q: %w", u.contentName, err)
		}

		if vmmac.Spec.Address == "" {
			vmmac.Spec.Address = vmmac.Status.Address
		}
		vmmacs = append(vmmacs, vmmac)
	}
	return vmmacs, nil
}

func (u *unifiedManifestReader) RestoreMACAddressOrder(ctx context.Context) ([]string, error) {
	vm, err := u.RestoreVirtualMachine(ctx)
	if err != nil {
		return nil, err
	}

	var macAddressOrder []string
	for _, ns := range vm.Status.Networks {
		switch ns.Type {
		case v1alpha2.NetworksTypeMain:
			macAddressOrder = append(macAddressOrder, "")
		default:
			macAddressOrder = append(macAddressOrder, ns.MAC)
		}
	}
	return macAddressOrder, nil
}

func (u *unifiedManifestReader) RestoreVirtualMachineBlockDeviceAttachments(ctx context.Context) ([]*v1alpha2.VirtualMachineBlockDeviceAttachment, error) {
	manifests, err := u.load(ctx)
	if err != nil {
		return nil, err
	}
	var vmbdas []*v1alpha2.VirtualMachineBlockDeviceAttachment
	for _, m := range manifests {
		if !m.Is(v1alpha2.SchemeGroupVersion.String(), v1alpha2.VirtualMachineBlockDeviceAttachmentKind) {
			continue
		}
		vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{}
		if err := json.Unmarshal(m.Raw, vmbda); err != nil {
			return nil, fmt.Errorf("unmarshal captured VirtualMachineBlockDeviceAttachment from SnapshotContent %q: %w", u.contentName, err)
		}
		vmbdas = append(vmbdas, vmbda)
	}
	return vmbdas, nil
}
