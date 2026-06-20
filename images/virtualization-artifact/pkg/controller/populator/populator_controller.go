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
	"fmt"
	"time"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	ControllerName = "populator-controller"

	PodVerbose    = "3"
	PodPullPolicy = string(corev1.PullIfNotPresent)

	requeueAfter        = time.Second
	pvcSupplementPrefix = "pvc"
)

type Reconciler struct {
	client client.Client
	pvc    *service.PersistentVolumeClaimService
}

func NewController(
	mgr manager.Manager,
	log *log.Logger,
	diskImporterImage string,
	requirements corev1.ResourceRequirements,
	dvcrSettings *dvcr.Settings,
) (controller.Controller, error) {
	reconciler := &Reconciler{
		client: mgr.GetClient(),
		pvc: service.NewPersistentVolumeClaimService(mgr.GetClient(), dvcrSettings, nil, service.DiskImporterConfig{
			Image:                diskImporterImage,
			ResourceRequirements: requirements,
			PullPolicy:           PodPullPolicy,
			Verbose:              PodVerbose,
		}),
	}

	ctr, err := builder.ControllerManagedBy(mgr).
		For(&corev1.PersistentVolumeClaim{}, builder.WithPredicates(predicate.NewPredicateFuncs(hasPopulationStrategy))).
		Build(reconciler)
	if err != nil {
		return nil, err
	}

	log.Info("Initialized PVC populator controller")
	return ctr, nil
}

func hasPopulationStrategy(obj client.Object) bool {
	return obj.GetAnnotations()[annotations.AnnPVCPopulationStrategy] != ""
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	pvc, err := object.FetchObject(ctx, req.NamespacedName, r.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("fetch pvc: %w", err)
	}
	if pvc == nil || pvc.Annotations[annotations.AnnPVCPopulationStrategy] == "" {
		return reconcile.Result{}, nil
	}

	strategy := pvc.Annotations[annotations.AnnPVCPopulationStrategy]
	if pvc.Annotations[annotations.AnnPVCPopulationDone] == "true" {
		return reconcile.Result{}, r.cleanup(ctx, pvc, strategy)
	}

	switch strategy {
	case service.PopulationStrategyCSIClone:
		return r.reconcileBoundOnly(ctx, pvc)
	case service.PopulationStrategySnapshot:
		if err := r.ensureSnapshot(ctx, pvc); err != nil {
			return reconcile.Result{}, err
		}
		res, err := r.reconcileBoundOnly(ctx, pvc)
		if err != nil || !res.IsZero() {
			return res, err
		}
		return reconcile.Result{}, r.cleanup(ctx, pvc, strategy)
	case service.PopulationStrategyDVCR, service.PopulationStrategyHostAssigned:
		return r.reconcileImporter(ctx, pvc, strategy)
	default:
		return reconcile.Result{}, nil
	}
}

func (r *Reconciler) reconcileBoundOnly(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (reconcile.Result, error) {
	if pvc.Status.Phase != corev1.ClaimBound {
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}
	return reconcile.Result{}, r.markDone(ctx, pvc)
}

func (r *Reconciler) reconcileImporter(ctx context.Context, pvc *corev1.PersistentVolumeClaim, strategy string) (reconcile.Result, error) {
	wffc, err := r.isWaitForFirstConsumer(ctx, pvc)
	if err != nil {
		return reconcile.Result{}, err
	}

	owner, sup, err := r.ownerAndSupplements(ctx, pvc)
	if err != nil {
		return reconcile.Result{}, err
	}
	if _, ok := owner.(*v1alpha2.VirtualDisk); ok && wffc && pvc.Annotations[service.SelectedNodeAnnotation] == "" {
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}
	source := sourceFromAnnotations(pvc, strategy, sup)
	if err := r.pvc.Import(ctx, pvc, source, owner, sup, nil); err != nil {
		return reconcile.Result{}, fmt.Errorf("import to pvc: %w", err)
	}

	phase, err := r.pvc.WaitForImport(ctx, pvc, source, owner, sup, nil)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("wait for pvc import: %w", err)
	}
	switch phase {
	case corev1.PodSucceeded:
		if err := r.markDone(ctx, pvc); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, r.cleanup(ctx, pvc, strategy)
	case corev1.PodFailed:
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}
}

func sourceFromAnnotations(pvc *corev1.PersistentVolumeClaim, strategy string, sup supplements.Generator) *service.PVCImportSource {
	switch strategy {
	case service.PopulationStrategyDVCR:
		secret := pvc.Annotations[annotations.AnnPVCPopulationSourceDVCRSecret]
		if secret == "" && sup != nil {
			secret = sup.DVCRAuthSecretForDV().Name
		}
		certConfigMap := pvc.Annotations[annotations.AnnPVCPopulationSourceDVCRCertConfigMap]
		if certConfigMap == "" && sup != nil {
			certConfigMap = sup.DVCRCABundleConfigMapForDV().Name
		}
		return service.NewPVCRegistryImportSource(
			pvc.Annotations[annotations.AnnPVCPopulationSourceDVCR],
			secret,
			certConfigMap,
		)
	case service.PopulationStrategyHostAssigned:
		namespace := pvc.Annotations[annotations.AnnPVCPopulationSourcePVCNamespace]
		if namespace == "" {
			namespace = pvc.Namespace
		}
		return service.NewPVCPVCImportSource(pvc.Annotations[annotations.AnnPVCPopulationSourcePVC], namespace)
	default:
		return nil
	}
}

func (r *Reconciler) ownerAndSupplements(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (client.Object, supplements.Generator, error) {
	for _, ref := range pvc.OwnerReferences {
		if ref.Kind == v1alpha2.VirtualDiskKind {
			vd, err := object.FetchObject(ctx, types.NamespacedName{Name: ref.Name, Namespace: pvc.Namespace}, r.client, &v1alpha2.VirtualDisk{})
			if err != nil {
				return nil, nil, fmt.Errorf("fetch owner virtualdisk: %w", err)
			}
			if vd == nil {
				return nil, nil, nil
			}
			return vd, supplements.NewGenerator(annotations.VDShortName, vd.Name, vd.Namespace, vd.UID), nil
		}
		if ref.Kind == v1alpha2.VirtualImageKind {
			vi, err := object.FetchObject(ctx, types.NamespacedName{Name: ref.Name, Namespace: pvc.Namespace}, r.client, &v1alpha2.VirtualImage{})
			if err != nil {
				return nil, nil, fmt.Errorf("fetch owner virtualimage: %w", err)
			}
			if vi == nil {
				return nil, nil, nil
			}
			return vi, supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID), nil
		}
	}
	return pvc, supplements.NewGenerator(pvcSupplementPrefix, pvc.Name, pvc.Namespace, pvc.UID), nil
}

func (r *Reconciler) markDone(ctx context.Context, pvc *corev1.PersistentVolumeClaim) error {
	latest := &corev1.PersistentVolumeClaim{}
	if err := r.client.Get(ctx, client.ObjectKeyFromObject(pvc), latest); err != nil {
		return err
	}
	patch := latest.DeepCopy()
	if patch.Annotations == nil {
		patch.Annotations = map[string]string{}
	}
	patch.Annotations[annotations.AnnPVCPopulationDone] = "true"
	patch.Annotations[annotations.AnnPVCImportPhase] = string(corev1.PodSucceeded)
	return r.client.Patch(ctx, patch, client.MergeFrom(latest))
}

func (r *Reconciler) cleanup(ctx context.Context, pvc *corev1.PersistentVolumeClaim, strategy string) error {
	_, sup, err := r.ownerAndSupplements(ctx, pvc)
	if err != nil {
		return err
	}
	if sup != nil {
		if _, err := r.pvc.Cleanup(ctx, sup, pvc); err != nil {
			return err
		}
	}
	if strategy == service.PopulationStrategySnapshot {
		snapshotName := pvc.Annotations[annotations.AnnPVCImportCloneSnapshot]
		if snapshotName != "" {
			err := r.client.Delete(ctx, &vsv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{Name: snapshotName, Namespace: pvc.Namespace}})
			if err != nil && !k8serrors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

func (r *Reconciler) ensureSnapshot(ctx context.Context, pvc *corev1.PersistentVolumeClaim) error {
	snapshotName := pvc.Annotations[annotations.AnnPVCImportCloneSnapshot]
	if snapshotName == "" && pvc.Spec.DataSourceRef != nil && pvc.Spec.DataSourceRef.Kind == "VolumeSnapshot" {
		snapshotName = pvc.Spec.DataSourceRef.Name
	}
	if snapshotName == "" && pvc.Spec.DataSource != nil && pvc.Spec.DataSource.Kind == "VolumeSnapshot" {
		snapshotName = pvc.Spec.DataSource.Name
	}
	if snapshotName == "" {
		return fmt.Errorf("snapshot population pvc %s/%s has no VolumeSnapshot name", pvc.Namespace, pvc.Name)
	}

	existing, err := object.FetchObject(ctx, types.NamespacedName{Name: snapshotName, Namespace: pvc.Namespace}, r.client, &vsv1.VolumeSnapshot{})
	if err != nil {
		return fmt.Errorf("fetch volume snapshot: %w", err)
	}
	if existing != nil {
		return nil
	}

	sourceName := pvc.Annotations[annotations.AnnPVCPopulationSourcePVC]
	if sourceName == "" {
		return fmt.Errorf("snapshot population pvc %s/%s has no source pvc annotation", pvc.Namespace, pvc.Name)
	}
	sourceNamespace := pvc.Annotations[annotations.AnnPVCPopulationSourcePVCNamespace]
	if sourceNamespace == "" {
		sourceNamespace = pvc.Namespace
	}
	sourcePVC, err := object.FetchObject(ctx, types.NamespacedName{Name: sourceName, Namespace: sourceNamespace}, r.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return fmt.Errorf("fetch source pvc: %w", err)
	}
	if sourcePVC == nil {
		return fmt.Errorf("source pvc %s/%s not found", sourceNamespace, sourceName)
	}

	snapshotClass, err := r.snapshotClassForPVC(ctx, sourcePVC)
	if err != nil {
		return err
	}

	vs := &vsv1.VolumeSnapshot{
		TypeMeta: metav1.TypeMeta{Kind: "VolumeSnapshot", APIVersion: "snapshot.storage.k8s.io/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            snapshotName,
			Namespace:       pvc.Namespace,
			OwnerReferences: []metav1.OwnerReference{service.MakeOwnerReference(sourcePVC)},
		},
		Spec: vsv1.VolumeSnapshotSpec{
			Source: vsv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: ptr.To(sourcePVC.Name),
			},
			VolumeSnapshotClassName: ptr.To(snapshotClass),
		},
	}
	if err := r.client.Create(ctx, vs); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create volume snapshot: %w", err)
	}
	return nil
}

func (r *Reconciler) snapshotClassForPVC(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (string, error) {
	storageClassName := ptr.Deref(pvc.Spec.StorageClassName, "")
	if storageClassName == "" {
		return "", fmt.Errorf("source pvc %s/%s has no storageClassName", pvc.Namespace, pvc.Name)
	}
	sc, err := object.FetchObject(ctx, types.NamespacedName{Name: storageClassName}, r.client, &storagev1.StorageClass{})
	if err != nil {
		return "", fmt.Errorf("fetch source storage class: %w", err)
	}
	if sc == nil {
		return "", fmt.Errorf("source storage class %q not found", storageClassName)
	}

	var list vsv1.VolumeSnapshotClassList
	if err := r.client.List(ctx, &list); err != nil {
		return "", fmt.Errorf("list volume snapshot classes: %w", err)
	}
	for _, item := range list.Items {
		if item.Driver == sc.Provisioner {
			return item.Name, nil
		}
	}
	return "", fmt.Errorf("no compatible VolumeSnapshotClass found for provisioner %q", sc.Provisioner)
}

func (r *Reconciler) isWaitForFirstConsumer(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (bool, error) {
	storageClassName := ptr.Deref(pvc.Spec.StorageClassName, "")
	if storageClassName == "" {
		return false, nil
	}
	sc, err := object.FetchObject(ctx, types.NamespacedName{Name: storageClassName}, r.client, &storagev1.StorageClass{})
	if err != nil {
		return false, fmt.Errorf("fetch storage class: %w", err)
	}
	return sc != nil && sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer, nil
}
