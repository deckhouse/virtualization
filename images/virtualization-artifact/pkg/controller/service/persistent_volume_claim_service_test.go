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
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonpvc "github.com/deckhouse/virtualization-controller/pkg/common/pvc"
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
	secret := "auth"
	cert := "ca"
	vd := diskImportTestVD()
	key := types.NamespacedName{Name: vd.Status.Target.PersistentVolumeClaim, Namespace: vd.Namespace}

	if err := svc.CreateTarget(ctx, key, sc.Name, resource.MustParse("1Gi"), NewPVCRegistryImportSource(url, secret, cert), vd, testVolumeModeGetter{}, nil); err != nil {
		t.Fatalf("CreateTarget failed: %v", err)
	}

	pvc := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, key, pvc); err != nil {
		t.Fatalf("target pvc not found: %v", err)
	}
	if got := pvc.Annotations[annotations.AnnPVCImportPhase]; got != string(corev1.PodPending) {
		t.Fatalf("unexpected import phase annotation: %q", got)
	}
	if len(pvc.OwnerReferences) != 1 || pvc.OwnerReferences[0].Kind != v1alpha2.VirtualDiskKind {
		t.Fatalf("target pvc owner reference not set: %#v", pvc.OwnerReferences)
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

	scratch := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, types.NamespacedName{Name: target.Name + "-scratch", Namespace: target.Namespace}, scratch); err != nil {
		t.Fatalf("get scratch pvc: %v", err)
	}
	if got, want := scratch.Spec.Resources.Requests[corev1.ResourceStorage], resource.MustParse("1342177280"); got.Cmp(want) != 0 {
		t.Fatalf("unexpected scratch size: got %s, want %s", got.String(), want.String())
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
	if err := c.Get(ctx, types.NamespacedName{Name: target.Name + "-scratch", Namespace: target.Namespace}, &corev1.PersistentVolumeClaim{}); client.IgnoreNotFound(err) == nil && err == nil {
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

	if err := svc.CreateTarget(ctx, client.ObjectKeyFromObject(target), sc.Name, resource.MustParse("1Gi"), NewPVCPVCImportSource(sourceClaim.Name, sourceClaim.Namespace), vd, testVolumeModeGetter{}, nil); err != nil {
		t.Fatalf("CreateTarget failed: %v", err)
	}

	created := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, created); err != nil {
		t.Fatalf("target pvc not found: %v", err)
	}
	if got := created.Annotations[annotations.AnnPVCImportCloneStrategy]; got != cloneStrategySnapshot {
		t.Fatalf("unexpected clone strategy: %q", got)
	}
	if got := created.Annotations[annotations.AnnPVCImportPhase]; got != string(corev1.PodPending) {
		t.Fatalf("unexpected import phase: %q", got)
	}
	if created.Spec.DataSourceRef == nil || created.Spec.DataSourceRef.Kind != "VolumeSnapshot" {
		t.Fatalf("target pvc does not reference VolumeSnapshot: %#v", created.Spec.DataSourceRef)
	}
	if got, want := created.Spec.Resources.Requests[corev1.ResourceStorage], resource.MustParse("2Gi"); got.Cmp(want) != 0 {
		t.Fatalf("unexpected target size: got %s, want %s", got.String(), want.String())
	}

	snapshot := &vsv1.VolumeSnapshot{}
	if err := c.Get(ctx, types.NamespacedName{Name: created.Annotations[annotations.AnnPVCImportCloneSnapshot], Namespace: vd.Namespace}, snapshot); err != nil {
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
	target.Annotations[annotations.AnnPVCImportCloneStrategy] = cloneStrategySnapshot
	target.Annotations[annotations.AnnPVCImportCloneSnapshot] = target.Name + "-clone-snapshot"
	target.Annotations[annotations.AnnPVCImportPhase] = string(corev1.PodPending)
	snapshot := &vsv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{Name: target.Annotations[annotations.AnnPVCImportCloneSnapshot], Namespace: target.Namespace}}
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
	refreshed := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(target), refreshed); err != nil {
		t.Fatalf("refresh target: %v", err)
	}
	if got := refreshed.Annotations[annotations.AnnPVCImportPhase]; got != string(corev1.PodSucceeded) {
		t.Fatalf("unexpected import phase: %q", got)
	}
}

func TestPVCServiceWaitForImportHostAssistedUsesQemuImgConvert(t *testing.T) {
	ctx := context.Background()
	vd := diskImportTestVD()
	target := diskImportTargetPVC(vd)
	importerPodName := diskImportImporterPodName(vd)
	target.Annotations[annotations.AnnPVCImportCloneStrategy] = cloneStrategyHost
	target.Annotations[annotations.AnnPVCImportPhase] = string(corev1.PodPending)
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

	pod := &corev1.Pod{}
	if err := c.Get(ctx, types.NamespacedName{Name: importerPodName, Namespace: target.Namespace}, pod); err != nil {
		t.Fatalf("get import pod: %v", err)
	}
	if len(pod.Spec.Containers) != 1 {
		t.Fatalf("unexpected containers: %#v", pod.Spec.Containers)
	}
	container := pod.Spec.Containers[0]
	if got := container.Command; len(got) != 1 || got[0] != "/usr/bin/qemu-img" {
		t.Fatalf("unexpected command: %#v", got)
	}
	wantArgs := []string{"convert", "-p", "-O", "raw", pvcImporterSourceBlockPath, pvcImporterWriteBlockPath}
	if len(container.Args) != len(wantArgs) {
		t.Fatalf("unexpected args: %#v", container.Args)
	}
	for i := range wantArgs {
		if container.Args[i] != wantArgs[i] {
			t.Fatalf("unexpected args: got %#v, want %#v", container.Args, wantArgs)
		}
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

	if err := svc.CreateTarget(ctx, client.ObjectKeyFromObject(target), sc.Name, resource.MustParse("1Gi"), NewPVCPVCImportSource(sourceClaim.Name, sourceClaim.Namespace), vd, testVolumeModeGetter{}, nil); err != nil {
		t.Fatalf("CreateTarget failed: %v", err)
	}

	created := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, created); err != nil {
		t.Fatalf("target pvc not found: %v", err)
	}
	if got := created.Annotations[annotations.AnnPVCImportCloneStrategy]; got != cloneStrategyCSI {
		t.Fatalf("unexpected clone strategy: %q", got)
	}
	if created.Spec.DataSourceRef == nil || created.Spec.DataSourceRef.Kind != "PersistentVolumeClaim" || created.Spec.DataSourceRef.Name != sourceClaim.Name {
		t.Fatalf("target pvc does not reference source PVC: %#v", created.Spec.DataSourceRef)
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
