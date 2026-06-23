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
	"testing"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonpvc "github.com/deckhouse/virtualization-controller/pkg/common/pvc"
	"github.com/deckhouse/virtualization-controller/pkg/controller/storageprofile"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func newTestPVCService(c client.Client) *PersistentVolumeClaimService {
	return NewPersistentVolumeClaimService(c, nil, nil, DiskImporterConfig{
		Image:      "disk-importer:latest",
		PullPolicy: string(corev1.PullIfNotPresent),
		Verbose:    "3",
	})
}

type testVolumeModeGetter struct{}

func (testVolumeModeGetter) GetVolumeAndAccessModes(_ context.Context, _ client.Object, _ *storagev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error) {
	return corev1.PersistentVolumeFilesystem, corev1.ReadWriteOnce, nil
}

func newTestVDSupplements(vd *v1alpha2.VirtualDisk) supplements.Generator {
	return &testVDSupplements{
		Generator: supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID),
		claimName: vd.Status.Target.PersistentVolumeClaim,
	}
}

type testVDSupplements struct {
	supplements.Generator
	claimName string
}

func (t *testVDSupplements) PersistentVolumeClaim() types.NamespacedName {
	return types.NamespacedName{Name: t.claimName, Namespace: t.Namespace()}
}

func (t *testVDSupplements) CommonResourceName() types.NamespacedName {
	return t.PersistentVolumeClaim()
}

func newTestTargetPVC(vd *v1alpha2.VirtualDisk, sc *storagev1.StorageClass, size resource.Quantity) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      vd.Status.Target.PersistentVolumeClaim,
			Namespace: vd.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         v1alpha2.SchemeGroupVersion.String(),
				Kind:               v1alpha2.VirtualDiskKind,
				Name:               vd.Name,
				UID:                vd.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			}},
		},
		Spec: *commonpvc.CreateSpec(&sc.Name, size, corev1.ReadWriteOnce, corev1.PersistentVolumeFilesystem),
	}
}

func TestPVCServiceCreateTargetCreatesPVCWithRegistrySource(t *testing.T) {
	ctx := context.Background()
	sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "fast"}}
	c := fake.NewClientBuilder().WithScheme(diskImportTestScheme(t)).WithObjects(sc).Build()
	svc := newTestPVCService(c)

	url := "docker://registry.example/image:tag"
	vd := diskImportTestVD()
	key := types.NamespacedName{Name: vd.Status.Target.PersistentVolumeClaim, Namespace: vd.Namespace}

	if _, err := svc.CreateTargetFromDVCR(ctx, key, sc.Name, ptr.To(resource.MustParse("1Gi")), vd, &PVCImportSourceRegistry{URL: url}, testVolumeModeGetter{}, nil); err != nil {
		t.Fatalf("CreateTargetFromDVCR failed: %v", err)
	}

	pvc := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, key, pvc); err != nil {
		t.Fatalf("target pvc not found: %v", err)
	}
	if len(pvc.OwnerReferences) != 1 || pvc.OwnerReferences[0].Kind != v1alpha2.VirtualDiskKind {
		t.Fatalf("target pvc owner reference not set: %#v", pvc.OwnerReferences)
	}
	if got := pvc.Annotations[annotations.AnnPVCPopulationStrategy]; got != PopulationStrategyDVCR {
		t.Fatalf("unexpected population strategy: %q", got)
	}
	if pvc.Spec.DataSourceRef == nil || ptr.Deref(pvc.Spec.DataSourceRef.APIGroup, "") != virtualizationAPIGroup || pvc.Spec.DataSourceRef.Kind != v1alpha2.VirtualDiskKind || pvc.Spec.DataSourceRef.Name != vd.Name {
		t.Fatalf("target pvc does not reference VirtualDisk populator source: %#v", pvc.Spec.DataSourceRef)
	}
	if got := pvc.Annotations[annotations.AnnPVCPopulationSourceDVCR]; got != url {
		t.Fatalf("unexpected dvcr source: %q", got)
	}
}

func TestPVCServiceWaitForImportIsResumable(t *testing.T) {
	ctx := context.Background()
	vd := diskImportTestVD()
	target := diskImportTargetPVC(vd)
	importerPodName := diskImportImporterPodName(vd)
	c := fake.NewClientBuilder().WithScheme(diskImportTestScheme(t)).WithObjects(target).Build()
	svc := newTestPVCService(c)
	sup := newTestVDSupplements(vd)

	if err := svc.Import(ctx, target, NewPVCRegistryImportSource("", "", ""), vd, sup, nil); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	phase, err := svc.WaitForImport(ctx, target, NewPVCRegistryImportSource("", "", ""), vd, sup, nil)
	if err != nil {
		t.Fatalf("WaitForImport failed: %v", err)
	}
	if phase != corev1.PodPending {
		t.Fatalf("unexpected phase after first WaitForImport: %s", phase)
	}

	pod := &corev1.Pod{}
	if err := c.Get(ctx, types.NamespacedName{Name: importerPodName, Namespace: target.Namespace}, pod); err != nil {
		t.Fatalf("get pod: %v", err)
	}

	// The importer fills the prime PVC, not the target.
	primeName := primePVCName(target)
	prime := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, types.NamespacedName{Name: primeName, Namespace: target.Namespace}, prime); err != nil {
		t.Fatalf("get prime pvc: %v", err)
	}
	if got := pod.Spec.Volumes; len(got) == 0 {
		t.Fatalf("importer pod has no volumes")
	}
	scratch := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, types.NamespacedName{Name: primeName + "-scratch", Namespace: target.Namespace}, scratch); err != nil {
		t.Fatalf("get scratch pvc: %v", err)
	}
	if got, want := scratch.Spec.Resources.Requests[corev1.ResourceStorage], resource.MustParse("1342177280"); got.Cmp(want) != 0 {
		t.Fatalf("unexpected scratch size: got %s, want %s", got.String(), want.String())
	}

	// Simulate the provisioner binding the prime PVC to a PersistentVolume.
	prime.Spec.VolumeName = "pv-prime"
	if err := c.Update(ctx, prime); err != nil {
		t.Fatalf("bind prime pvc: %v", err)
	}
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-prime"},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			ClaimRef: &corev1.ObjectReference{
				Kind: "PersistentVolumeClaim", Namespace: target.Namespace, Name: prime.Name, UID: prime.UID,
			},
		},
	}
	if err := c.Create(ctx, pv); err != nil {
		t.Fatalf("create prime pv: %v", err)
	}

	pod.Status.Phase = corev1.PodSucceeded
	if err := c.Status().Update(ctx, pod); err != nil {
		t.Fatalf("update pod status: %v", err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(target), target); err != nil {
		t.Fatalf("refresh target: %v", err)
	}

	phase, err = svc.WaitForImport(ctx, target, NewPVCRegistryImportSource("", "", ""), vd, sup, nil)
	if err != nil {
		t.Fatalf("WaitForImport after pod success failed: %v", err)
	}
	if phase != corev1.PodSucceeded {
		t.Fatalf("unexpected final phase: %s", phase)
	}

	// The prime's volume must have been rebound to the target.
	refreshedTarget := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(target), refreshedTarget); err != nil {
		t.Fatalf("refresh target: %v", err)
	}
	if refreshedTarget.Spec.VolumeName != "pv-prime" {
		t.Fatalf("target was not rebound to prime's PV: volumeName=%q", refreshedTarget.Spec.VolumeName)
	}

	// Helper resources must be cleaned up.
	if err := c.Get(ctx, types.NamespacedName{Name: primeName, Namespace: target.Namespace}, &corev1.PersistentVolumeClaim{}); client.IgnoreNotFound(err) == nil && err == nil {
		t.Fatalf("prime pvc still exists")
	}
	if err := c.Get(ctx, types.NamespacedName{Name: primeName + "-scratch", Namespace: target.Namespace}, &corev1.PersistentVolumeClaim{}); client.IgnoreNotFound(err) == nil && err == nil {
		t.Fatalf("scratch pvc still exists")
	}
	if err := c.Get(ctx, types.NamespacedName{Name: importerPodName, Namespace: target.Namespace}, &corev1.Pod{}); client.IgnoreNotFound(err) == nil && err == nil {
		t.Fatalf("import pod still exists")
	}
}

func TestPVCServiceCreateTargetPicksVolumeSnapshotStrategyWhenPossible(t *testing.T) {
	ctx := context.Background()
	vd := diskImportTestVD()
	sc := diskImportStorageClass()
	sourceClaim := diskImportSourcePVC()
	snapshotClass := &vsv1.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{Name: "snap-fast"},
		Driver:     sc.Provisioner,
	}
	c := fake.NewClientBuilder().WithScheme(diskImportTestScheme(t)).WithObjects(sc, sourceClaim, snapshotClass).Build()
	svc := newTestPVCService(c)
	target := newTestTargetPVC(vd, sc, resource.MustParse("1Gi"))

	if _, err := svc.CreateTargetFromPVC(ctx, client.ObjectKeyFromObject(target), sc.Name, ptr.To(resource.MustParse("1Gi")), vd, sourceClaim, testVolumeModeGetter{}, nil); err != nil {
		t.Fatalf("CreateTargetFromPVC failed: %v", err)
	}

	created := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, created); err != nil {
		t.Fatalf("target pvc not found: %v", err)
	}
	if got := created.Annotations[annotations.AnnPVCPopulationStrategy]; got != PopulationStrategySnapshot {
		t.Fatalf("unexpected population strategy: %q", got)
	}
	if created.Spec.DataSourceRef == nil || created.Spec.DataSourceRef.Kind != "VolumeSnapshot" {
		t.Fatalf("target pvc does not reference VolumeSnapshot: %#v", created.Spec.DataSourceRef)
	}
	if got, want := created.Spec.Resources.Requests[corev1.ResourceStorage], resource.MustParse("2Gi"); got.Cmp(want) != 0 {
		t.Fatalf("unexpected target size: got %s, want %s", got.String(), want.String())
	}

	snapshot := &vsv1.VolumeSnapshot{}
	if err := c.Get(ctx, types.NamespacedName{Name: created.Spec.DataSourceRef.Name, Namespace: vd.Namespace}, snapshot); err != nil {
		t.Fatalf("clone snapshot not found: %v", err)
	}
	if snapshot.Spec.Source.PersistentVolumeClaimName == nil || *snapshot.Spec.Source.PersistentVolumeClaimName != sourceClaim.Name {
		t.Fatalf("unexpected snapshot source: %#v", snapshot.Spec.Source.PersistentVolumeClaimName)
	}
}

func TestPVCServiceWaitForImportSmartCloneMarksSucceededAndCleansSnapshot(t *testing.T) {
	ctx := context.Background()
	vd := diskImportTestVD()
	target := diskImportTargetPVC(vd)
	target.Annotations[annotations.AnnPVCPopulationStrategy] = PopulationStrategySnapshot
	target.Spec.DataSourceRef = &corev1.TypedObjectReference{APIGroup: ptr.To("snapshot.storage.k8s.io"), Kind: "VolumeSnapshot", Name: target.Name + "-clone-snapshot"}
	snapshot := &vsv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{Name: target.Spec.DataSourceRef.Name, Namespace: target.Namespace}}
	c := fake.NewClientBuilder().WithScheme(diskImportTestScheme(t)).WithObjects(target, snapshot).Build()
	svc := newTestPVCService(c)
	sup := newTestVDSupplements(vd)

	phase, err := svc.WaitForImport(ctx, target, NewPVCPVCImportSource("source", "default"), vd, sup, nil)
	if err != nil {
		t.Fatalf("WaitForImport failed: %v", err)
	}
	if phase != corev1.PodSucceeded {
		t.Fatalf("unexpected phase: %s", phase)
	}
	if err := c.Get(ctx, types.NamespacedName{Name: snapshot.Name, Namespace: snapshot.Namespace}, &vsv1.VolumeSnapshot{}); client.IgnoreNotFound(err) == nil && err == nil {
		t.Fatalf("clone snapshot still exists")
	}
}

func TestPVCServiceWaitForImportHostAssistedUsesQemuImgConvert(t *testing.T) {
	ctx := context.Background()
	vd := diskImportTestVD()
	target := diskImportTargetPVC(vd)
	target.Annotations[annotations.AnnPVCPopulationStrategy] = PopulationStrategyHostAssigned
	target.Spec.VolumeMode = ptr.To(corev1.PersistentVolumeBlock)
	sourceClaim := diskImportSourcePVC()
	sourceClaim.Spec.VolumeMode = ptr.To(corev1.PersistentVolumeBlock)
	c := fake.NewClientBuilder().WithScheme(diskImportTestScheme(t)).WithObjects(target, sourceClaim).Build()
	svc := newTestPVCService(c)
	sup := newTestVDSupplements(vd)

	if err := svc.Import(ctx, target, NewPVCPVCImportSource(sourceClaim.Name, sourceClaim.Namespace), vd, sup, nil); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	phase, err := svc.WaitForImport(ctx, target, NewPVCPVCImportSource(sourceClaim.Name, sourceClaim.Namespace), vd, sup, nil)
	if err != nil {
		t.Fatalf("WaitForImport failed: %v", err)
	}
	if phase != corev1.PodPending {
		t.Fatalf("unexpected phase: %s", phase)
	}

	sourcePod := &corev1.Pod{}
	if err := c.Get(ctx, sup.PVCSourceImporterPod(), sourcePod); err != nil {
		t.Fatalf("get source import pod: %v", err)
	}
	targetPod := &corev1.Pod{}
	if err := c.Get(ctx, sup.PVCTargetImporterPod(), targetPod); err != nil {
		t.Fatalf("get target import pod: %v", err)
	}
	if err := c.Get(ctx, types.NamespacedName{Name: primePVCName(target) + "-scratch", Namespace: target.Namespace}, &corev1.PersistentVolumeClaim{}); !k8serrors.IsNotFound(err) {
		t.Fatalf("host-assigned import must not create scratch pvc, got error: %v", err)
	}
	sourceContainer := sourcePod.Spec.Containers[0]
	if got := sourceContainer.Command; len(got) != 1 || got[0] != "/usr/sbin/nbdkit" {
		t.Fatalf("unexpected source command: %#v", got)
	}
	container := targetPod.Spec.Containers[0]
	if got := container.Command; len(got) != 1 || got[0] != "/usr/bin/pvc-target-importer" {
		t.Fatalf("unexpected command: %#v", got)
	}
	wantNBD := "nbd://" + sup.PVCSourceImporterService().Name + ":10809"
	var gotOwnerUID, gotNBD string
	for _, env := range container.Env {
		switch env.Name {
		case common.OwnerUID:
			gotOwnerUID = env.Value
		case common.ImporterNBDEndpoint:
			gotNBD = env.Value
		}
	}
	if gotOwnerUID != string(vd.UID) {
		t.Fatalf("unexpected owner UID env: got %q, want %q", gotOwnerUID, vd.UID)
	}
	if gotNBD != wantNBD {
		t.Fatalf("unexpected NBD endpoint env: got %q, want %q", gotNBD, wantNBD)
	}
	if len(container.Ports) != 1 || container.Ports[0].Name != "metrics" || container.Ports[0].ContainerPort != 8443 {
		t.Fatalf("unexpected metrics port: %#v", container.Ports)
	}
}

func TestPVCServiceCreateTargetFallsBackToCSIClone(t *testing.T) {
	ctx := context.Background()
	vd := diskImportTestVD()
	sc := diskImportStorageClass()
	sourceClaim := diskImportSourcePVC()
	c := fake.NewClientBuilder().WithScheme(diskImportTestScheme(t)).WithObjects(sc, sourceClaim).Build()
	svc := newTestPVCService(c)
	target := newTestTargetPVC(vd, sc, resource.MustParse("1Gi"))

	if _, err := svc.CreateTargetFromPVC(ctx, client.ObjectKeyFromObject(target), sc.Name, ptr.To(resource.MustParse("1Gi")), vd, sourceClaim, testVolumeModeGetter{}, nil); err != nil {
		t.Fatalf("CreateTargetFromPVC failed: %v", err)
	}

	created := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, created); err != nil {
		t.Fatalf("target pvc not found: %v", err)
	}
	if got := created.Annotations[annotations.AnnPVCPopulationStrategy]; got != PopulationStrategyCSIClone {
		t.Fatalf("unexpected population strategy: %q", got)
	}
	if created.Spec.DataSourceRef == nil || created.Spec.DataSourceRef.Kind != "PersistentVolumeClaim" || created.Spec.DataSourceRef.Name != sourceClaim.Name {
		t.Fatalf("target pvc does not reference source PVC: %#v", created.Spec.DataSourceRef)
	}
}

func TestPVCServiceCreateTargetFromPVCUsesSnapshotForSDSReplicated(t *testing.T) {
	ctx := context.Background()
	vd := diskImportTestVD()
	sc := diskImportStorageClass()
	sc.Provisioner = storageprofile.SDSReplicatedCSIProvisioner
	sourceClaim := diskImportSourcePVC()
	snapshotClass := &vsv1.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{Name: "sds-replicated-volume"},
		Driver:     sc.Provisioner,
	}
	snapshot := cdiv1.CloneStrategySnapshot
	sp := &cdiv1.StorageProfile{
		ObjectMeta: metav1.ObjectMeta{Name: sc.Name},
		Status: cdiv1.StorageProfileStatus{
			CloneStrategy: &snapshot,
		},
	}
	c := fake.NewClientBuilder().WithScheme(diskImportTestScheme(t)).WithObjects(sc, sourceClaim, snapshotClass, sp).Build()
	svc := newTestPVCService(c)
	target := newTestTargetPVC(vd, sc, resource.MustParse("1Gi"))

	if _, err := svc.CreateTargetFromPVC(ctx, client.ObjectKeyFromObject(target), sc.Name, ptr.To(resource.MustParse("1Gi")), vd, sourceClaim, testVolumeModeGetter{}, nil); err != nil {
		t.Fatalf("CreateTargetFromPVC failed: %v", err)
	}

	created := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, created); err != nil {
		t.Fatalf("target pvc not found: %v", err)
	}
	if got := created.Annotations[annotations.AnnPVCPopulationStrategy]; got != PopulationStrategySnapshot {
		t.Fatalf("unexpected population strategy: %q", got)
	}
	if got := created.Annotations[annotations.AnnPVCPopulationSourcePVC]; got != sourceClaim.Name {
		t.Fatalf("unexpected source pvc: %q", got)
	}
	if created.Spec.DataSourceRef == nil || created.Spec.DataSourceRef.Kind != "VolumeSnapshot" {
		t.Fatalf("target pvc does not reference VolumeSnapshot: %#v", created.Spec.DataSourceRef)
	}
	snapshotObj := &vsv1.VolumeSnapshot{}
	if err := c.Get(ctx, types.NamespacedName{Name: created.Spec.DataSourceRef.Name, Namespace: vd.Namespace}, snapshotObj); err != nil {
		t.Fatalf("clone snapshot not found: %v", err)
	}
}

func diskImportTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := storagev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := netv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := vsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := cdiv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func diskImportStorageClass() *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fast",
			Annotations: map[string]string{
				annotations.AnnVirtualDiskVolumeMode: string(corev1.PersistentVolumeFilesystem),
				annotations.AnnVirtualDiskAccessMode: string(corev1.ReadWriteOnce),
			},
		},
		Provisioner: "csi.example.com",
	}
}

func diskImportSourcePVC() *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "source", Namespace: "default"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To("fast"),
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			VolumeMode:       ptr.To(corev1.PersistentVolumeFilesystem),
			Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("2Gi"),
			},
		},
	}
}

func diskImportTestVD() *v1alpha2.VirtualDisk {
	return &v1alpha2.VirtualDisk{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualDiskKind},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "disk",
			Namespace: "default",
			UID:       "22222222-2222-2222-2222-222222222222",
		},
		Status: v1alpha2.VirtualDiskStatus{Target: v1alpha2.DiskTarget{PersistentVolumeClaim: "d8v-vd-22222222-2222-2222-2222-222222222222-abcde"}},
	}
}

func diskImportImporterPodName(vd *v1alpha2.VirtualDisk) string {
	return "d8v-vd-pvc-importer-" + string(vd.UID)
}

func diskImportTargetPVC(vd *v1alpha2.VirtualDisk) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        vd.Status.Target.PersistentVolumeClaim,
			Namespace:   vd.Namespace,
			UID:         "33333333-3333-3333-3333-333333333333",
			Annotations: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualDiskKind,
				Name:       vd.Name,
				UID:        vd.UID,
				Controller: ptr.To(true),
			}},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To("fast"),
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			VolumeMode:       ptr.To(corev1.PersistentVolumeFilesystem),
			Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
}
