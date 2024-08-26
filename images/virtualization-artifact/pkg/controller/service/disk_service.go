/*
Copyright 2024 Flant JSC

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
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	storev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dvutil "github.com/deckhouse/virtualization-controller/pkg/common/datavolume"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

type DiskService struct {
	client       client.Client
	dvcrSettings *dvcr.Settings
	protection   *ProtectionService
}

func NewDiskService(
	client client.Client,
	dvcrSettings *dvcr.Settings,
	protection *ProtectionService,
) *DiskService {
	return &DiskService{
		client:       client,
		dvcrSettings: dvcrSettings,
		protection:   protection,
	}
}

func (s DiskService) Start(
	ctx context.Context,
	pvcSize resource.Quantity,
	storageClass *string,
	source *cdiv1.DataVolumeSource,
	obj ObjectKind,
	sup *supplements.Generator,
	wffc bool,
) error {
	dvBuilder := kvbuilder.NewDV(sup.DataVolume())
	dvBuilder.SetDataSource(source)
	dvBuilder.SetPVC(storageClass, pvcSize)
	dvBuilder.SetOwnerRef(obj, obj.GroupVersionKind())
	dvBuilder.SetBindingMode(wffc)

	// WaitForFirstConsumer is mainly needed for local storage.
	// To prevent virtual machine migrations from failing, we set PVC to RWO so that virtual machines definitely cannot migrate.
	if wffc {
		dvBuilder.SetAccessMode(corev1.ReadWriteOnce)
	}

	err := s.client.Create(ctx, dvBuilder.GetResource())
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	if source.Blank != nil {
		return nil
	}

	return supplements.EnsureForDataVolume(ctx, s.client, sup, dvBuilder.GetResource(), s.dvcrSettings)
}

func (s DiskService) CleanUp(ctx context.Context, sup *supplements.Generator) (bool, error) {
	subResourcesHaveDeleted, err := s.CleanUpSupplements(ctx, sup)
	if err != nil {
		return false, err
	}

	pvc, err := s.GetPersistentVolumeClaim(ctx, sup)
	if err != nil {
		return false, err
	}

	var resourcesHaveDeleted bool

	if pvc != nil {
		resourcesHaveDeleted = true

		err = s.protection.RemoveProtection(ctx, pvc)
		if err != nil {
			return false, err
		}

		err = s.client.Delete(ctx, pvc)
		if err != nil && !k8serrors.IsNotFound(err) {
			return false, err
		}
	}

	return resourcesHaveDeleted || subResourcesHaveDeleted, nil
}

func (s DiskService) CleanUpSupplements(ctx context.Context, sup *supplements.Generator) (bool, error) {
	dv, err := s.GetDataVolume(ctx, sup)
	if err != nil {
		return false, err
	}

	var hasDeleted bool

	if dv != nil {
		hasDeleted = true
		err = s.protection.RemoveProtection(ctx, dv)
		if err != nil {
			return false, err
		}

		err = s.client.Delete(ctx, dv)
		if err != nil && !k8serrors.IsNotFound(err) {
			return false, err
		}

		var pvc *corev1.PersistentVolumeClaim
		pvc, err = s.GetPersistentVolumeClaim(ctx, sup)
		if err != nil {
			return false, err
		}
		if pvc != nil {
			pvc.ObjectMeta.OwnerReferences = slices.DeleteFunc(pvc.ObjectMeta.OwnerReferences, func(ref metav1.OwnerReference) bool {
				return ref.Kind == "DataVolume"
			})
			err = s.client.Update(ctx, pvc)
			if err != nil && !k8serrors.IsNotFound(err) {
				return false, err
			}
		}
	}

	return hasDeleted, supplements.CleanupForDataVolume(ctx, s.client, sup, s.dvcrSettings)
}

func (s DiskService) Protect(ctx context.Context, owner client.Object, dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim) error {
	err := s.protection.AddOwnerRef(ctx, owner, pvc)
	if err != nil {
		return fmt.Errorf("failed to add owner ref for pvc: %w", err)
	}

	err = s.protection.AddProtection(ctx, dv, pvc)
	if err != nil {
		return fmt.Errorf("failed to add protection for disk's supplements: %w", err)
	}

	return nil
}

func (s DiskService) Unprotect(ctx context.Context, dv *cdiv1.DataVolume) error {
	err := s.protection.RemoveProtection(ctx, dv)
	if err != nil {
		return fmt.Errorf("failed to remove protection for disk's supplements: %w", err)
	}

	return nil
}

func (s DiskService) Resize(ctx context.Context, pvc *corev1.PersistentVolumeClaim, newSize resource.Quantity) error {
	if pvc == nil {
		return errors.New("got nil pvc")
	}

	curSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]

	// newSize <= curSize
	if newSize.Cmp(curSize) != 1 {
		return fmt.Errorf("new pvc %s/%s size %s is too low: should be > %s", pvc.Namespace, pvc.Name, newSize.String(), curSize.String())
	}

	pvc.Spec.Resources.Requests[corev1.ResourceStorage] = newSize

	err := s.client.Update(ctx, pvc)
	if err != nil {
		return fmt.Errorf("failed to increase size for pvc %s/%s from %s to %s : %w", pvc.Namespace, pvc.Name, curSize.String(), newSize.String(), err)
	}

	return nil
}

func (s DiskService) IsImportDone(dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim) bool {
	return common.IsDataVolumeComplete(dv) && common.IsPVCBound(pvc)
}

func (s DiskService) GetProgress(dv *cdiv1.DataVolume, prevProgress string, opts ...GetProgressOption) string {
	if dv == nil {
		return prevProgress
	}

	dvProgress := string(dv.Status.Progress)
	if dvProgress != "N/A" && dvProgress != "" {
		for _, o := range opts {
			dvProgress = o.Apply(dvProgress)
		}

		return dvProgress
	}

	return prevProgress
}

func (s DiskService) GetCapacity(pvc *corev1.PersistentVolumeClaim) string {
	if pvc != nil && pvc.Status.Phase == corev1.ClaimBound {
		return util.GetPointer(pvc.Status.Capacity[corev1.ResourceStorage]).String()
	}

	return ""
}

func (s DiskService) GetDataVolume(ctx context.Context, sup *supplements.Generator) (*cdiv1.DataVolume, error) {
	return helper.FetchObject(ctx, sup.DataVolume(), s.client, &cdiv1.DataVolume{})
}

func (s DiskService) GetPersistentVolumeClaim(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
	return helper.FetchObject(ctx, sup.PersistentVolumeClaim(), s.client, &corev1.PersistentVolumeClaim{})
}

func (s DiskService) CheckImportProcess(ctx context.Context, dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim, storageClassName *string) error {
	var err error

	if storageClassName == nil || *storageClassName == "" {
		err = s.checkDefaultStorageClass(ctx)
	} else {
		err = s.checkStorageClass(ctx, *storageClassName)
	}
	if err != nil {
		return err
	}

	if dv == nil {
		return nil
	}

	dvRunning := GetDataVolumeCondition(cdiv1.DataVolumeRunning, dv.Status.Conditions)
	if dvRunning == nil || dvRunning.Status != corev1.ConditionFalse {
		return nil
	}

	if strings.Contains(dvRunning.Reason, "Error") {
		return fmt.Errorf("%w: %s", ErrDataVolumeNotRunning, dvRunning.Message)
	}

	if pvc == nil {
		return nil
	}

	key := types.NamespacedName{
		Namespace: dv.Namespace,
		Name:      dvutil.GetImporterPrimeName(pvc.UID),
	}

	cdiImporterPrime, err := helper.FetchObject(ctx, key, s.client, &corev1.Pod{})
	if err != nil {
		return err
	}

	if cdiImporterPrime != nil {
		podInitializedCond, ok := GetPodCondition(corev1.PodInitialized, cdiImporterPrime.Status.Conditions)
		if ok && podInitializedCond.Status == corev1.ConditionFalse && strings.Contains(podInitializedCond.Reason, "Error") {
			return fmt.Errorf("%w; %s error %s: %s", ErrDataVolumeNotRunning, key.String(), podInitializedCond.Reason, podInitializedCond.Message)
		}

		podScheduledCond, ok := GetPodCondition(corev1.PodScheduled, cdiImporterPrime.Status.Conditions)
		if ok && podScheduledCond.Status == corev1.ConditionFalse && (podScheduledCond.Reason == corev1.PodReasonUnschedulable || strings.Contains(podScheduledCond.Reason, "Error")) {
			return fmt.Errorf("%w; %s error %s: %s", ErrDataVolumeNotRunning, key.String(), podScheduledCond.Reason, podScheduledCond.Message)
		}
	}

	return nil
}

func (s DiskService) checkDefaultStorageClass(ctx context.Context) error {
	var scs storev1.StorageClassList
	err := s.client.List(ctx, &scs, &client.ListOptions{})
	if err != nil {
		return err
	}

	for _, sc := range scs.Items {
		if sc.Annotations[common.AnnDefaultStorageClass] == "true" {
			return nil
		}
	}

	return ErrDefaultStorageClassNotFound
}

func (s DiskService) checkStorageClass(ctx context.Context, storageClassName string) error {
	var sc storev1.StorageClass
	err := s.client.Get(ctx, types.NamespacedName{
		Name: storageClassName,
	}, &sc, &client.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ErrStorageClassNotFound
		}

		return err
	}

	return nil
}

var ErrInsufficientPVCSize = errors.New("the specified pvc size is insufficient")

func GetValidatedPVCSize(pvcSize *resource.Quantity, requiredSize resource.Quantity) (resource.Quantity, error) {
	if requiredSize.IsZero() {
		return resource.Quantity{}, errors.New("got zero size from data source, please report a bug")
	}

	if pvcSize == nil {
		return requiredSize, nil
	}

	if pvcSize.IsZero() {
		return resource.Quantity{}, errors.New("cannot create disk with zero pvc size")
	}

	switch pvcSize.Cmp(requiredSize) {
	case -1:
		specPart := strconv.FormatUint(uint64(pvcSize.Value()), 10)
		if specPart != pvcSize.String() {
			specPart += fmt.Sprintf(" (%s)", pvcSize.String())
		}

		return resource.Quantity{}, fmt.Errorf("%w: %s < %d", ErrInsufficientPVCSize, specPart, requiredSize.Value())
	case 1:
		return *pvcSize, nil
	default:
		return requiredSize, nil
	}
}
