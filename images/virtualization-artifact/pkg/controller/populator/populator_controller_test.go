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

package populator

import (
	"context"
	"testing"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestPopulatorMarksBoundCSICloneDone(t *testing.T) {
	ctx := context.Background()
	vd := testVD()
	pvc := testTargetPVC(vd)
	pvc.Annotations[annotations.AnnPVCPopulationStrategy] = service.PopulationStrategyCSIClone
	pvc.Status.Phase = corev1.ClaimBound

	c := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithStatusSubresource(&corev1.PersistentVolumeClaim{}).
		WithObjects(vd, pvc).
		Build()
	r := testReconciler(c)

	if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(pvc)}); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	refreshed := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(pvc), refreshed); err != nil {
		t.Fatalf("get pvc: %v", err)
	}
	if got := refreshed.Annotations[annotations.AnnPVCPopulationDone]; got != "true" {
		t.Fatalf("unexpected done annotation: %q", got)
	}
}

func TestPopulatorStartsDVCRImport(t *testing.T) {
	ctx := context.Background()
	vd := testVD()
	pvc := testTargetPVC(vd)
	pvc.Annotations[annotations.AnnPVCPopulationStrategy] = service.PopulationStrategyDVCR
	pvc.Annotations[annotations.AnnPVCPopulationSourceDVCR] = "docker://registry.example/disk:tag"

	c := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(vd, pvc).
		Build()
	r := testReconciler(c)

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(pvc)})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatalf("expected requeue while importer is pending")
	}

	pod := &corev1.Pod{}
	if err := c.Get(ctx, types.NamespacedName{Name: "d8v-vd-pvc-importer-" + string(vd.UID), Namespace: vd.Namespace}, pod); err != nil {
		t.Fatalf("expected pvc-importer pod: %v", err)
	}
	if len(pod.OwnerReferences) != 1 || pod.OwnerReferences[0].Kind != "PersistentVolumeClaim" || pod.OwnerReferences[0].Name != pvc.Name {
		t.Fatalf("unexpected pod owner references: %#v", pod.OwnerReferences)
	}

	prime := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, types.NamespacedName{Name: pvc.Name + "-prime", Namespace: pvc.Namespace}, prime); err != nil {
		t.Fatalf("expected prime pvc: %v", err)
	}
}

func TestPopulatorStartsVirtualImageWFFCDVCRImportWithoutSelectedNode(t *testing.T) {
	ctx := context.Background()
	vi := testVI()
	pvc := testVITargetPVC(vi)
	pvc.Annotations[annotations.AnnPVCPopulationStrategy] = service.PopulationStrategyDVCR
	pvc.Annotations[annotations.AnnPVCPopulationSourceDVCR] = "docker://registry.example/image:tag"
	sc := &storagev1.StorageClass{
		ObjectMeta:        metav1.ObjectMeta{Name: "fast"},
		VolumeBindingMode: ptr.To(storagev1.VolumeBindingWaitForFirstConsumer),
	}

	c := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(vi, pvc, sc).
		Build()
	r := testReconciler(c)

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(pvc)})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatalf("expected requeue while importer is pending")
	}

	pod := &corev1.Pod{}
	if err := c.Get(ctx, types.NamespacedName{Name: "d8v-vi-pvc-importer-" + string(vi.UID), Namespace: vi.Namespace}, pod); err != nil {
		t.Fatalf("expected pvc-importer pod: %v", err)
	}
}

func TestPopulatorStartsStandaloneDVCRImport(t *testing.T) {
	ctx := context.Background()
	pvc := testStandaloneTargetPVC("target", "default")
	pvc.Annotations[annotations.AnnPVCPopulationStrategy] = service.PopulationStrategyDVCR
	pvc.Annotations[annotations.AnnPVCPopulationSourceDVCR] = "docker://registry.example/disk:tag"

	c := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(pvc).
		Build()
	r := testReconciler(c)

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(pvc)})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatalf("expected requeue while importer is pending")
	}

	pod := &corev1.Pod{}
	if err := c.Get(ctx, types.NamespacedName{Name: "d8v-pvc-pvc-importer-" + string(pvc.UID), Namespace: pvc.Namespace}, pod); err != nil {
		t.Fatalf("expected standalone pvc-importer pod: %v", err)
	}
	if len(pod.OwnerReferences) != 1 || pod.OwnerReferences[0].Kind != "PersistentVolumeClaim" || pod.OwnerReferences[0].Name != pvc.Name {
		t.Fatalf("unexpected pod owner references: %#v", pod.OwnerReferences)
	}

	sup := supplements.NewGenerator(pvcSupplementPrefix, pvc.Name, pvc.Namespace, pvc.UID)
	assertContainerUsesDVCRSupplement(t, pod.Spec.Containers[0], sup)
}

func TestPopulatorStartsHostAssignedPVCCloneWithSourceAndTargetPods(t *testing.T) {
	ctx := context.Background()
	vd := testVD()
	source := testSourcePVC("source", vd.Namespace)
	source.Spec.VolumeMode = ptr.To(corev1.PersistentVolumeBlock)
	target := testTargetPVC(vd)
	target.Annotations[annotations.AnnPVCPopulationStrategy] = service.PopulationStrategyHostAssigned
	target.Annotations[annotations.AnnPVCPopulationSourcePVC] = source.Name
	target.Spec.VolumeMode = ptr.To(corev1.PersistentVolumeBlock)

	c := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(vd, source, target).
		Build()
	r := testReconciler(c)
	sup := supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID)

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(target)})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatalf("expected requeue while source importer is pending")
	}

	sourcePod := &corev1.Pod{}
	if err := c.Get(ctx, sup.PVCSourceImporterPod(), sourcePod); err != nil {
		t.Fatalf("expected pvc-source-importer pod: %v", err)
	}
	if got := sourcePod.Spec.Containers[0].Command; len(got) != 1 || got[0] != "/usr/sbin/nbdkit" {
		t.Fatalf("unexpected source pod command: %#v", got)
	}
	if err := c.Get(ctx, sup.PVCTargetImporterPod(), &corev1.Pod{}); client.IgnoreNotFound(err) == nil && err == nil {
		t.Fatalf("target importer pod must wait for source pod IP")
	}
	if err := c.Get(ctx, sup.PVCImporterPod(), &corev1.Pod{}); client.IgnoreNotFound(err) == nil && err == nil {
		t.Fatalf("host-assigned clone must not create legacy pvc-importer pod")
	}

	sourcePod.Status.Phase = corev1.PodRunning
	sourcePod.Status.PodIP = "10.0.0.20"
	sourcePod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	if err := c.Status().Update(ctx, sourcePod); err != nil {
		t.Fatalf("update source pod status: %v", err)
	}

	result, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(target)})
	if err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatalf("expected requeue while target importer is pending")
	}

	targetPod := &corev1.Pod{}
	if err := c.Get(ctx, sup.PVCTargetImporterPod(), targetPod); err != nil {
		t.Fatalf("expected pvc-target-importer pod: %v", err)
	}
	container := targetPod.Spec.Containers[0]
	if got := container.Command; len(got) != 1 || got[0] != "/usr/bin/qemu-img" {
		t.Fatalf("unexpected target pod command: %#v", got)
	}
	wantArgs := []string{"convert", "-p", "-O", "raw", "nbd://10.0.0.20:10809", "/dev/pvc-importer-block-volume"}
	if len(container.Args) != len(wantArgs) {
		t.Fatalf("unexpected target pod args: %#v", container.Args)
	}
	for i := range wantArgs {
		if container.Args[i] != wantArgs[i] {
			t.Fatalf("unexpected target pod args: got %#v, want %#v", container.Args, wantArgs)
		}
	}
	if err := c.Get(ctx, types.NamespacedName{Name: target.Name + "-prime-scratch", Namespace: target.Namespace}, &corev1.PersistentVolumeClaim{}); client.IgnoreNotFound(err) == nil && err == nil {
		t.Fatalf("host-assigned clone must not create scratch pvc")
	}
}

func TestPopulatorCreatesMissingSnapshot(t *testing.T) {
	ctx := context.Background()
	vd := testVD()
	source := testSourcePVC("source", vd.Namespace)
	target := testTargetPVC(vd)
	target.Annotations[annotations.AnnPVCPopulationStrategy] = service.PopulationStrategySnapshot
	target.Annotations[annotations.AnnPVCPopulationSourcePVC] = source.Name
	target.Spec.DataSource = &corev1.TypedLocalObjectReference{APIGroup: ptr.To("snapshot.storage.k8s.io"), Kind: "VolumeSnapshot", Name: "target-snapshot"}
	target.Spec.DataSourceRef = &corev1.TypedObjectReference{APIGroup: ptr.To("snapshot.storage.k8s.io"), Kind: "VolumeSnapshot", Name: "target-snapshot"}
	sc := &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "fast"}, Provisioner: "csi.example.com"}
	vsc := &vsv1.VolumeSnapshotClass{ObjectMeta: metav1.ObjectMeta{Name: "snap-fast"}, Driver: "csi.example.com"}

	c := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(vd, source, target, sc, vsc).
		Build()
	r := testReconciler(c)

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(target)})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Fatalf("expected requeue while snapshot target is pending")
	}

	snapshot := &vsv1.VolumeSnapshot{}
	if err := c.Get(ctx, types.NamespacedName{Name: "target-snapshot", Namespace: target.Namespace}, snapshot); err != nil {
		t.Fatalf("expected volume snapshot: %v", err)
	}
	if snapshot.Spec.Source.PersistentVolumeClaimName == nil || *snapshot.Spec.Source.PersistentVolumeClaimName != source.Name {
		t.Fatalf("unexpected snapshot source: %#v", snapshot.Spec.Source.PersistentVolumeClaimName)
	}
	if len(snapshot.OwnerReferences) != 1 || snapshot.OwnerReferences[0].Kind != "PersistentVolumeClaim" || snapshot.OwnerReferences[0].Name != source.Name {
		t.Fatalf("unexpected snapshot owner references: %#v", snapshot.OwnerReferences)
	}
}

func testStandaloneTargetPVC(name, namespace string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			UID:         "55555555-5555-5555-5555-555555555555",
			Annotations: map[string]string{},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To("fast"),
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			VolumeMode:       ptr.To(corev1.PersistentVolumeFilesystem),
			Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}},
		},
	}
}

func testReconciler(c client.Client) *Reconciler {
	return &Reconciler{
		client: c,
		pvc: service.NewPersistentVolumeClaimService(c, nil, nil, service.DiskImporterConfig{
			Image:      "pvc-importer:latest",
			PullPolicy: string(corev1.PullIfNotPresent),
			Verbose:    "1",
		}),
	}
}

func assertContainerUsesDVCRSupplement(t *testing.T, container corev1.Container, sup supplements.Generator) {
	t.Helper()

	certName := sup.DVCRCABundleConfigMapForDV().Name
	var hasCertDir bool
	var hasAuthSecret bool
	for _, env := range container.Env {
		if env.Name == common.ImporterCertDirVar {
			hasCertDir = true
		}
		if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil && env.ValueFrom.SecretKeyRef.Name == sup.DVCRAuthSecretForDV().Name {
			hasAuthSecret = true
		}
	}
	if !hasCertDir {
		t.Fatalf("expected importer container to use DVCR CA configmap %q", certName)
	}
	if !hasAuthSecret {
		t.Fatalf("expected importer container to use DVCR auth secret %q", sup.DVCRAuthSecretForDV().Name)
	}
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		corev1.AddToScheme,
		netv1.AddToScheme,
		storagev1.AddToScheme,
		vsv1.AddToScheme,
		v1alpha2.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	return scheme
}

func testVD() *v1alpha2.VirtualDisk {
	return &v1alpha2.VirtualDisk{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualDiskKind},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "disk",
			Namespace: "default",
			UID:       "22222222-2222-2222-2222-222222222222",
		},
	}
}

func testVI() *v1alpha2.VirtualImage {
	return &v1alpha2.VirtualImage{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha2.SchemeGroupVersion.String(), Kind: v1alpha2.VirtualImageKind},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "image",
			Namespace: "default",
			UID:       "66666666-6666-6666-6666-666666666666",
		},
	}
}

func testTargetPVC(vd *v1alpha2.VirtualDisk) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "target",
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
	}
}

func testVITargetPVC(vi *v1alpha2.VirtualImage) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "target",
			Namespace:   vi.Namespace,
			UID:         "77777777-7777-7777-7777-777777777777",
			Annotations: map[string]string{},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha2.SchemeGroupVersion.String(),
				Kind:       v1alpha2.VirtualImageKind,
				Name:       vi.Name,
				UID:        vi.UID,
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
	}
}

func testSourcePVC(name, namespace string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "44444444-4444-4444-4444-444444444444",
		},
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
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			},
		},
	}
}
