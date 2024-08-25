package service

import (
	"context"
	"fmt"
	"slices"
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
)

type PVCCloneService struct {
	client       client.Client
	protection   *ProtectionService
	dvcrSettings *dvcr.Settings
}

func NewPVCCloneService(
	client client.Client,
	protection *ProtectionService,
) *PVCCloneService {
	return &PVCCloneService{
		client:     client,
		protection: protection,
	}
}

// TODO сверить с DISK_SERVICE - возможно разницы особо нет

func (s PVCCloneService) Start(
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

	return nil
}

func (s PVCCloneService) GetDataVolume(ctx context.Context, sup *supplements.Generator) (*cdiv1.DataVolume, error) {
	return helper.FetchObject(ctx, sup.DataVolume(), s.client, &cdiv1.DataVolume{})
}

func (s PVCCloneService) GetPersistentVolumeClaim(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
	return helper.FetchObject(ctx, sup.PersistentVolumeClaim(), s.client, &corev1.PersistentVolumeClaim{})
}

func (s PVCCloneService) GetPersistentVolume(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolume, error) {
	if pvc == nil {
		return nil, nil
	}

	return helper.FetchObject(ctx, types.NamespacedName{Name: pvc.Spec.VolumeName}, s.client, &corev1.PersistentVolume{})
}

func (s PVCCloneService) Protect(ctx context.Context, owner client.Object, dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim, pv *corev1.PersistentVolume) error {
	err := s.protection.AddOwnerRef(ctx, owner, pvc)
	if err != nil {
		return fmt.Errorf("failed to add owner ref for pvc: %w", err)
	}

	err = s.protection.AddProtection(ctx, dv, pvc, pv)
	if err != nil {
		return fmt.Errorf("failed to add protection for disk's supplements: %w", err)
	} // TODO fix message

	return nil
}

func (s PVCCloneService) Unprotect(ctx context.Context, dv *cdiv1.DataVolume) error {
	err := s.protection.RemoveProtection(ctx, dv)
	if err != nil {
		return fmt.Errorf("failed to remove protection for disk's supplements: %w", err)
	} // TODO fix message

	return nil
}

func (s PVCCloneService) IsImportDone(dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim) bool {
	return common.IsDataVolumeComplete(dv) && common.IsPVCBound(pvc)
}

func (s PVCCloneService) GetProgress(dv *cdiv1.DataVolume, prevProgress string, opts ...GetProgressOption) string {
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

func (s PVCCloneService) CheckImportProcess(ctx context.Context, dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim, storageClassName *string) error {
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

func (s PVCCloneService) checkDefaultStorageClass(ctx context.Context) error {
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

func (s PVCCloneService) checkStorageClass(ctx context.Context, storageClassName string) error {
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

func (s PVCCloneService) CleanUp(ctx context.Context, sup *supplements.Generator) (bool, error) {
	subResourcesHaveDeleted, err := s.CleanUpSupplements(ctx, sup)
	if err != nil {
		return false, err
	}

	pvc, err := s.GetPersistentVolumeClaim(ctx, sup)
	if err != nil {
		return false, err
	}
	pv, err := s.GetPersistentVolume(ctx, pvc)
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

	if pv != nil {
		resourcesHaveDeleted = true

		err = s.protection.RemoveProtection(ctx, pv)
		if err != nil {
			return false, err
		}

		err = s.client.Delete(ctx, pv)
		if err != nil && !k8serrors.IsNotFound(err) {
			return false, err
		}
	}

	return resourcesHaveDeleted || subResourcesHaveDeleted, nil
}

func (s PVCCloneService) CleanUpSupplements(ctx context.Context, sup *supplements.Generator) (bool, error) {
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
		pvc, err := s.GetPersistentVolumeClaim(ctx, sup)
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
