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

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	ControllerName = "populator-controller"

	PodVerbose    = "3"
	PodPullPolicy = string(corev1.PullIfNotPresent)

	pvcSupplementPrefix = "pvc"
)

type Reconciler struct {
	client client.Client
	pvc    *service.PersistentVolumeClaimService
	log    *log.Logger
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
		log:    log,
		pvc: service.NewPersistentVolumeClaimService(mgr.GetClient(), dvcrSettings, nil, service.DiskImporterConfig{
			Image:                diskImporterImage,
			ResourceRequirements: requirements,
			PullPolicy:           PodPullPolicy,
			Verbose:              PodVerbose,
		}),
	}

	ctr, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:     reconciler,
		LogConstructor: logger.NewConstructor(log),
	})
	if err != nil {
		return nil, err
	}

	if err := addWatches(mgr, ctr); err != nil {
		return nil, err
	}

	log.Info("Initialized PVC populator controller")
	return ctr, nil
}

func hasPopulationStrategy(obj client.Object) bool {
	return obj.GetAnnotations()[annotations.AnnPVCPopulationStrategy] != ""
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	res, err := r.reconcile(ctx, req)
	if err != nil && k8serrors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
		// The namespace is being deleted: admission rejects every create in it and
		// the PVC will be garbage-collected along with the namespace, so retrying
		// only floods the log with Reconciler errors.
		r.log.Info("PVC population canceled: namespace is terminating",
			"namespace", req.Namespace,
			"pvc", req.Name,
			"err", err.Error(),
		)
		return reconcile.Result{}, nil
	}
	return res, err
}

func (r *Reconciler) reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
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
		_, err := r.reconcileBoundOnly(ctx, pvc)
		return reconcile.Result{}, err
	case service.PopulationStrategySnapshot:
		if err := r.ensureSnapshot(ctx, pvc); err != nil {
			return reconcile.Result{}, err
		}
		marked, err := r.reconcileBoundOnly(ctx, pvc)
		if err != nil {
			return reconcile.Result{}, err
		}
		if !marked {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, r.cleanup(ctx, pvc, strategy)
	case service.PopulationStrategyDVCR, service.PopulationStrategyHostAssigned:
		return r.reconcileImporter(ctx, pvc, strategy)
	default:
		return reconcile.Result{}, nil
	}
}

func (r *Reconciler) reconcileBoundOnly(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (bool, error) {
	if pvc.Status.Phase != corev1.ClaimBound {
		return false, nil
	}
	return true, r.markDone(ctx, pvc)
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
		r.log.Info("PVC population waiting for WaitForFirstConsumer node selection",
			"namespace", pvc.Namespace,
			"pvc", pvc.Name,
			"strategy", strategy,
		)
		return reconcile.Result{}, nil
	}

	beforePods, err := r.importerPodSnapshots(ctx, sup, strategy)
	if err != nil {
		return reconcile.Result{}, err
	}

	source := sourceFromAnnotations(pvc, strategy, sup)

	// The importer pod is pinned to the node the volume is provisioned on (the
	// consuming VirtualMachine's node), so it must also carry the VM and class
	// tolerations: without them the pod can never be scheduled when the VM sits
	// on a tainted node (e.g. a control-plane one).
	nodePlacement, err := r.ownerNodePlacement(ctx, owner)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := r.pvc.Import(ctx, pvc, source, owner, sup, nodePlacement); err != nil {
		return reconcile.Result{}, fmt.Errorf("import to pvc: %w", err)
	}

	afterImportPods, err := r.importerPodSnapshots(ctx, sup, strategy)
	if err != nil {
		return reconcile.Result{}, err
	}
	r.logImporterPodTransitions(pvc, strategy, beforePods, afterImportPods)

	rebindStarted := r.rebindPending(ctx, pvc, sup, strategy)
	if rebindStarted {
		r.log.Info("PVC population rebind started",
			"namespace", pvc.Namespace,
			"pvc", pvc.Name,
			"strategy", strategy,
		)
	}

	phase, err := r.pvc.WaitForImport(ctx, pvc, source, owner, sup, nodePlacement)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("wait for pvc import: %w", err)
	}

	afterWaitPods, err := r.importerPodSnapshots(ctx, sup, strategy)
	if err != nil {
		return reconcile.Result{}, err
	}
	r.logImporterPodTransitions(pvc, strategy, afterImportPods, afterWaitPods)

	switch phase {
	case corev1.PodSucceeded:
		if rebindStarted {
			r.log.Info("PVC population rebind finished",
				"namespace", pvc.Namespace,
				"pvc", pvc.Name,
				"strategy", strategy,
			)
		}
		if err := r.markDone(ctx, pvc); err != nil {
			return reconcile.Result{}, err
		}
		r.log.Info("PVC population finished",
			"namespace", pvc.Namespace,
			"pvc", pvc.Name,
			"strategy", strategy,
		)
		return reconcile.Result{}, r.cleanup(ctx, pvc, strategy)
	case corev1.PodFailed:
		r.log.Info("PVC population importer pod failed",
			"namespace", pvc.Namespace,
			"pvc", pvc.Name,
			"strategy", strategy,
		)
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, nil
	}
}

func sourceFromAnnotations(pvc *corev1.PersistentVolumeClaim, strategy string, sup supplements.Generator) *service.PVCImportSource {
	switch strategy {
	case service.PopulationStrategyDVCR:
		secret := ""
		certConfigMap := ""
		if sup != nil {
			secret = sup.DVCRAuthSecretForDV().Name
			certConfigMap = sup.DVCRCABundleConfigMapForDV().Name
		}
		return service.NewPVCRegistryImportSource(
			pvc.Annotations[annotations.AnnPVCPopulationSourceDVCR],
			secret,
			certConfigMap,
		)
	case service.PopulationStrategyHostAssigned:
		return service.NewPVCPVCImportSource(pvc.Annotations[annotations.AnnPVCPopulationSourcePVC], pvc.Namespace)
	default:
		return nil
	}
}

// ownerNodePlacement resolves the node placement (tolerations of the consuming
// VirtualMachine and its class) for the importer helpers of the PVC owner.
// Only VirtualDisks are consumed by VirtualMachines; other owners get none.
func (r *Reconciler) ownerNodePlacement(ctx context.Context, owner client.Object) (*provisioner.NodePlacement, error) {
	vd, ok := owner.(*v1alpha2.VirtualDisk)
	if !ok {
		return nil, nil
	}
	nodePlacement, err := commonvd.GetNodePlacement(ctx, r.client, vd)
	if err != nil {
		return nil, fmt.Errorf("get node placement for virtual disk %s/%s: %w", vd.Namespace, vd.Name, err)
	}
	return nodePlacement, nil
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
		snapshotName := snapshotNameFromPVC(pvc)
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
	snapshotName := snapshotNameFromPVC(pvc)
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
	sourceNamespace := pvc.Namespace
	sourcePVC, err := object.FetchObject(ctx, types.NamespacedName{Name: sourceName, Namespace: sourceNamespace}, r.client, &corev1.PersistentVolumeClaim{})
	if err != nil {
		return fmt.Errorf("fetch source pvc: %w", err)
	}
	if sourcePVC == nil {
		return fmt.Errorf("source pvc %s/%s not found", sourceNamespace, sourceName)
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
		},
	}
	if err := r.client.Create(ctx, vs); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create volume snapshot: %w", err)
	}
	r.log.Info("PVC population snapshot created",
		"namespace", pvc.Namespace,
		"pvc", pvc.Name,
		"snapshot", snapshotName,
	)
	return nil
}

func snapshotNameFromPVC(pvc *corev1.PersistentVolumeClaim) string {
	if pvc == nil {
		return ""
	}
	if pvc.Spec.DataSourceRef != nil && pvc.Spec.DataSourceRef.Kind == "VolumeSnapshot" {
		return pvc.Spec.DataSourceRef.Name
	}
	if pvc.Spec.DataSource != nil && pvc.Spec.DataSource.Kind == "VolumeSnapshot" {
		return pvc.Spec.DataSource.Name
	}
	return ""
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
