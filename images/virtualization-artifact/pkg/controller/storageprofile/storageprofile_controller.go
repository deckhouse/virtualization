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

package storageprofile

import (
	"context"
	"fmt"
	"reflect"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

const ControllerName = "storageprofile-controller"

type Reconciler struct {
	client client.Client
	log    *log.Logger
}

func NewController(mgr manager.Manager, log *log.Logger) (controller.Controller, error) {
	reconciler := &Reconciler{client: mgr.GetClient(), log: log}
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
	log.Info("Initialized StorageProfile controller")
	return ctr, nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	sc := &storagev1.StorageClass{}
	if err := r.client.Get(ctx, req.NamespacedName, sc); err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, r.deleteStorageProfile(ctx, req.Name)
		}
		return reconcile.Result{}, err
	}
	if sc.DeletionTimestamp != nil {
		return reconcile.Result{}, r.deleteStorageProfile(ctx, req.Name)
	}
	return reconcile.Result{}, r.reconcileStorageProfile(ctx, sc)
}

func (r *Reconciler) reconcileStorageProfile(ctx context.Context, sc *storagev1.StorageClass) error {
	profile := &cdiv1.StorageProfile{}
	var previous *cdiv1.StorageProfile
	if err := r.client.Get(ctx, types.NamespacedName{Name: sc.Name}, profile); err != nil {
		if !k8serrors.IsNotFound(err) {
			return err
		}
		profile = emptyStorageProfile(sc.Name)
	} else {
		previous = profile.DeepCopy()
	}

	profile.Status.StorageClass = &sc.Name
	profile.Status.Provisioner = &sc.Provisioner
	snapshotClass := r.snapshotClassForProvisioner(ctx, sc.Provisioner, profile.Spec.SnapshotClass)
	if snapshotClass == "" {
		profile.Status.SnapshotClass = nil
	} else {
		profile.Status.SnapshotClass = &snapshotClass
	}
	profile.Status.CloneStrategy = reconcileCloneStrategy(sc, profile.Spec.CloneStrategy, snapshotClass)
	profile.Status.DataImportCronSourceFormat = reconcileDataImportCronSourceFormat(profile.Spec.DataImportCronSourceFormat)
	if len(profile.Spec.ClaimPropertySets) > 0 {
		profile.Status.ClaimPropertySets = profile.Spec.ClaimPropertySets
	} else {
		profile.Status.ClaimPropertySets = defaultClaimPropertySets()
	}

	if previous == nil {
		return r.client.Create(ctx, profile)
	}
	if !reflect.DeepEqual(previous, profile) {
		return r.client.Update(ctx, profile)
	}
	return nil
}

func (r *Reconciler) deleteStorageProfile(ctx context.Context, name string) error {
	err := r.client.Delete(ctx, &cdiv1.StorageProfile{ObjectMeta: metav1.ObjectMeta{Name: name}})
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *Reconciler) snapshotClassForProvisioner(ctx context.Context, provisioner string, desired *string) string {
	var list vsv1.VolumeSnapshotClassList
	if err := r.client.List(ctx, &list); err != nil {
		return ""
	}
	if desired != nil && *desired != "" {
		for _, item := range list.Items {
			if item.Name == *desired && item.Driver == provisioner {
				return item.Name
			}
		}
		return ""
	}
	for _, item := range list.Items {
		if item.Driver == provisioner {
			return item.Name
		}
	}
	return ""
}

func reconcileCloneStrategy(sc *storagev1.StorageClass, desired *cdiv1.CDICloneStrategy, snapshotClass string) *cdiv1.CDICloneStrategy {
	if desired != nil {
		return desired
	}
	if value, ok := sc.Annotations["cdi.kubevirt.io/clone-strategy"]; ok {
		switch value {
		case "copy":
			strategy := cdiv1.CloneStrategyHostAssisted
			return &strategy
		case "snapshot":
			strategy := cdiv1.CloneStrategySnapshot
			return &strategy
		case "csi-clone":
			strategy := cdiv1.CloneStrategyCsiClone
			return &strategy
		}
	}
	if snapshotClass != "" {
		strategy := cdiv1.CloneStrategySnapshot
		return &strategy
	}
	strategy := cdiv1.CloneStrategyHostAssisted
	return &strategy
}

func reconcileDataImportCronSourceFormat(desired *cdiv1.DataImportCronSourceFormat) *cdiv1.DataImportCronSourceFormat {
	if desired != nil {
		return desired
	}
	format := cdiv1.DataImportCronSourceFormatPvc
	return &format
}

func defaultClaimPropertySets() []cdiv1.ClaimPropertySet {
	fs := corev1.PersistentVolumeFilesystem
	block := corev1.PersistentVolumeBlock
	return []cdiv1.ClaimPropertySet{
		{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, VolumeMode: &fs},
		{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, VolumeMode: &block},
	}
}

func emptyStorageProfile(name string) *cdiv1.StorageProfile {
	return &cdiv1.StorageProfile{
		TypeMeta: metav1.TypeMeta{Kind: "StorageProfile", APIVersion: "cdi.kubevirt.io/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "virtualization-controller",
			},
		},
	}
}

func addWatches(mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &storagev1.StorageClass{}, &handler.TypedEnqueueRequestForObject[*storagev1.StorageClass]{})); err != nil {
		return err
	}
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &cdiv1.StorageProfile{}, &handler.TypedEnqueueRequestForObject[*cdiv1.StorageProfile]{})); err != nil {
		return err
	}
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &corev1.PersistentVolume{},
		handler.TypedEnqueueRequestsFromMapFunc(func(_ context.Context, pv *corev1.PersistentVolume) []reconcile.Request {
			if pv.Spec.StorageClassName == "" {
				return nil
			}
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: pv.Spec.StorageClassName}}}
		}),
		predicate.TypedFuncs[*corev1.PersistentVolume]{
			CreateFunc: func(e event.TypedCreateEvent[*corev1.PersistentVolume]) bool {
				return e.Object.Spec.StorageClassName != "" && e.Object.Spec.HostPath != nil
			},
			UpdateFunc: func(e event.TypedUpdateEvent[*corev1.PersistentVolume]) bool {
				return e.ObjectNew.Spec.StorageClassName != "" && e.ObjectNew.Spec.HostPath != nil
			},
			DeleteFunc: func(e event.TypedDeleteEvent[*corev1.PersistentVolume]) bool {
				return e.Object.Spec.StorageClassName != "" && e.Object.Spec.HostPath != nil
			},
		},
	)); err != nil {
		return err
	}
	if err := ctr.Watch(source.Kind(mgr.GetCache(), &vsv1.VolumeSnapshotClass{},
		handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, vsc *vsv1.VolumeSnapshotClass) []reconcile.Request {
			var scs storagev1.StorageClassList
			if err := mgr.GetClient().List(ctx, &scs); err != nil {
				ctr.GetLogger().Error(err, "Unable to list StorageClasses")
				return nil
			}
			var requests []reconcile.Request
			for _, sc := range scs.Items {
				if sc.Provisioner == vsc.Driver {
					requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: sc.Name}})
				}
			}
			return requests
		}),
	)); err != nil {
		return fmt.Errorf("watch VolumeSnapshotClass: %w", err)
	}
	return nil
}
