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
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	dvutil "github.com/deckhouse/virtualization-controller/pkg/common/datavolume"
	networkpolicy "github.com/deckhouse/virtualization-controller/pkg/common/network_policy"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/pointer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type DiskService struct {
	client         client.Client
	dvcrSettings   *dvcr.Settings
	protection     *ProtectionService
	controllerName string
}

func NewDiskService(
	client client.Client,
	dvcrSettings *dvcr.Settings,
	protection *ProtectionService,
	controllerName string,
) *DiskService {
	return &DiskService{
		client:         client,
		dvcrSettings:   dvcrSettings,
		protection:     protection,
		controllerName: controllerName,
	}
}

func (s DiskService) Start(
	ctx context.Context,
	pvcSize resource.Quantity,
	sc *storagev1.StorageClass,
	source *cdiv1.DataVolumeSource,
	obj client.Object,
	sup supplements.DataVolumeSupplement,
	opts ...Option,
) error {
	if sc == nil {
		return errors.New("cannot create DataVolume: StorageClass must not be nil")
	}

	options := newGenericOptions(opts...)

	dvBuilder := kvbuilder.NewDV(sup.DataVolume())
	dvBuilder.SetDataSource(source)
	dvBuilder.SetOwnerRef(obj, obj.GetObjectKind().GroupVersionKind())

	if options.nodePlacement != nil {
		err := dvBuilder.SetNodePlacement(options.nodePlacement)
		if err != nil {
			return fmt.Errorf("set node placement: %w", err)
		}
	}

	volumeMode, accessMode, err := s.GetVolumeAndAccessModes(ctx, obj, sc)
	if err != nil {
		return fmt.Errorf("get volume and access modes: %w", err)
	}

	dvBuilder.SetPVC(&sc.Name, pvcSize, accessMode, volumeMode)

	if s.isImmediateBindingMode(sc) {
		dvBuilder.SetImmediate()
	}

	dv := dvBuilder.GetResource()
	err = s.client.Create(ctx, dv)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	err = networkpolicy.CreateNetworkPolicy(ctx, s.client, dv, sup, s.protection.GetFinalizer())
	if err != nil {
		return fmt.Errorf("failed to create NetworkPolicy: %w", err)
	}

	if source.Blank != nil || source.PVC != nil {
		return nil
	}

	return supplements.EnsureForDataVolume(ctx, s.client, sup, dvBuilder.GetResource(), s.dvcrSettings)
}

func (s DiskService) GetVolumeAndAccessModes(ctx context.Context, obj client.Object, sc *storagev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error) {
	if obj == nil {
		return "", "", errors.New("object is nil")
	}
	if sc == nil {
		return "", "", errors.New("storage class is nil")
	}

	// Priority: object > storage class > storage profile.

	// 1. Get modes from annotations on the object.
	accessMode, _ := s.parseAccessMode(obj)
	volumeMode, _ := s.parseVolumeMode(obj)

	if accessMode != "" && volumeMode != "" {
		return volumeMode, accessMode, nil
	}

	// 2. Get modes from annotations on the storage class.
	if m, exists := s.parseAccessMode(sc); accessMode == "" && exists {
		accessMode = m
	}
	if m, exists := s.parseVolumeMode(sc); volumeMode == "" && exists {
		volumeMode = m
	}

	if accessMode != "" && volumeMode != "" {
		return volumeMode, accessMode, nil
	}

	// 3. Get modes from storage profile.
	storageProfile, err := s.GetStorageProfile(ctx, sc.Name)
	if err != nil {
		return "", "", fmt.Errorf("get storage profile: %w", err)
	}

	if storageProfile == nil {
		return "", "", fmt.Errorf("storage profile %q not found: %w", sc.Name, ErrStorageProfileNotFound)
	}

	storageCaps := s.parseStorageCapabilities(storageProfile.Status)
	if accessMode == "" && storageCaps.AccessMode != "" {
		accessMode = storageCaps.AccessMode
	}
	if volumeMode == "" && storageCaps.VolumeMode != "" {
		volumeMode = storageCaps.VolumeMode
	}

	return volumeMode, accessMode, nil
}

func (s DiskService) StartImmediate(
	ctx context.Context,
	pvcSize resource.Quantity,
	sc *storagev1.StorageClass,
	source *cdiv1.DataVolumeSource,
	obj client.Object,
	dataVolumeSupplement supplements.DataVolumeSupplement,
) error {
	if sc == nil {
		return errors.New("cannot create DataVolume: StorageClass must not be nil")
	}

	dvBuilder := kvbuilder.NewDV(dataVolumeSupplement.DataVolume())
	dvBuilder.SetDataSource(source)
	dvBuilder.SetOwnerRef(obj, obj.GetObjectKind().GroupVersionKind())
	dvBuilder.SetPVC(ptr.To(sc.GetName()), pvcSize, corev1.ReadWriteMany, corev1.PersistentVolumeBlock)
	dvBuilder.SetImmediate()
	dv := dvBuilder.GetResource()

	err := s.client.Create(ctx, dv)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	err = networkpolicy.CreateNetworkPolicy(ctx, s.client, dv, dataVolumeSupplement, s.protection.GetFinalizer())
	if err != nil {
		return fmt.Errorf("failed to create NetworkPolicy: %w", err)
	}

	if source.PVC != nil {
		return nil
	}

	return supplements.EnsureForDataVolume(ctx, s.client, dataVolumeSupplement, dvBuilder.GetResource(), s.dvcrSettings)
}

func (s DiskService) CheckProvisioning(ctx context.Context, pvc *corev1.PersistentVolumeClaim) error {
	if pvc == nil || pvc.Status.Phase == corev1.ClaimBound {
		return nil
	}

	podName, ok := pvc.Annotations[annotations.AnnProvisionerName]
	if !ok || podName == "" {
		return nil
	}

	pod, err := object.FetchObject(ctx, types.NamespacedName{Name: podName, Namespace: pvc.Namespace}, s.client, &corev1.Pod{})
	if err != nil {
		return fmt.Errorf("failed to fetch data volume provisioner %s: %w", podName, err)
	}

	if pod == nil {
		return nil
	}

	scheduled, _ := conditions.GetPodCondition(corev1.PodScheduled, pod.Status.Conditions)
	if scheduled.Status == corev1.ConditionFalse && scheduled.Reason == corev1.PodReasonUnschedulable {
		return ErrDataVolumeProvisionerUnschedulable
	}

	return nil
}

func (s DiskService) CreatePersistentVolumeClaim(ctx context.Context, pvc *corev1.PersistentVolumeClaim) error {
	err := s.client.Create(ctx, pvc)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func (s DiskService) CleanUp(ctx context.Context, sup supplements.Generator) (bool, error) {
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

func (s DiskService) CleanUpSupplements(ctx context.Context, sup supplements.Generator) (bool, error) {
	// 1. Update owner ref of pvc.
	pvc, err := s.GetPersistentVolumeClaim(ctx, sup)
	if err != nil {
		return false, err
	}

	if pvc != nil {
		ownerReferences := slices.DeleteFunc(pvc.OwnerReferences, func(ref metav1.OwnerReference) bool {
			return ref.Kind == "DataVolume"
		})

		if len(pvc.OwnerReferences) != len(ownerReferences) {
			pvc.ObjectMeta.OwnerReferences = ownerReferences
			err = s.client.Update(ctx, pvc)
			if err != nil && !k8serrors.IsNotFound(err) {
				return false, fmt.Errorf("update owner ref of pvc: %w", err)
			}
		}
	}

	// 2. Delete network policy.
	networkPolicy, err := networkpolicy.GetNetworkPolicy(ctx, s.client, sup.LegacyDataVolume(), sup)
	if err != nil {
		return false, err
	}

	if networkPolicy != nil {
		err = s.protection.RemoveProtection(ctx, networkPolicy)
		if err != nil {
			return false, fmt.Errorf("remove protection from network policy: %w", err)
		}

		err = s.client.Delete(ctx, networkPolicy)
		if err != nil && !k8serrors.IsNotFound(err) {
			return false, fmt.Errorf("delete network policy: %w", err)
		}
	}

	// 3. Delete DataVolume.
	var hasDeleted bool
	dv, err := s.GetDataVolume(ctx, sup)
	if err != nil {
		return false, fmt.Errorf("get dv: %w", err)
	}

	if dv != nil {
		err = s.protection.RemoveProtection(ctx, dv)
		if err != nil {
			return false, fmt.Errorf("remove protection from dv: %w", err)
		}

		err = s.client.Delete(ctx, dv)
		if err != nil && !k8serrors.IsNotFound(err) {
			return false, fmt.Errorf("delete dv: %w", err)
		}

		hasDeleted = true
	}

	return hasDeleted, supplements.CleanupForDataVolume(ctx, s.client, sup, s.dvcrSettings)
}

func (s DiskService) Protect(ctx context.Context, sup supplements.Generator, owner client.Object, dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim) error {
	err := s.protection.AddOwnerRef(ctx, owner, pvc)
	if err != nil {
		return fmt.Errorf("failed to add owner ref for pvc: %w", err)
	}

	err = s.protection.AddProtection(ctx, dv, pvc)
	if err != nil {
		return fmt.Errorf("failed to add protection for disk's supplements: %w", err)
	}

	if dv != nil {
		networkPolicy, err := networkpolicy.GetNetworkPolicyFromObject(ctx, s.client, dv, sup)
		if err != nil {
			return fmt.Errorf("failed to get networkPolicy for disk's supplements protection: %w", err)
		}

		if networkPolicy != nil {
			err = s.protection.AddProtection(ctx, networkPolicy)
			if err != nil {
				return fmt.Errorf("failed to remove protection for disk's supplements: %w", err)
			}
		}
	}

	return nil
}

func (s DiskService) Unprotect(ctx context.Context, sup supplements.Generator, dv *cdiv1.DataVolume) error {
	err := s.protection.RemoveProtection(ctx, dv)
	if err != nil {
		return fmt.Errorf("failed to remove protection for disk's supplements: %w", err)
	}

	if dv != nil {
		networkPolicy, err := networkpolicy.GetNetworkPolicyFromObject(ctx, s.client, dv, sup)
		if err != nil {
			return fmt.Errorf("failed to get networkPolicy for removing disk's supplements protection: %w", err)
		}

		if networkPolicy != nil {
			err = s.protection.RemoveProtection(ctx, networkPolicy)
			if err != nil {
				return fmt.Errorf("failed to remove protection for disk's supplements: %w", err)
			}
		}
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
	return dv != nil && dv.Status.Phase == cdiv1.Succeeded && pvc != nil && pvc.Status.Phase == corev1.ClaimBound
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
		return pointer.GetPointer(pvc.Status.Capacity[corev1.ResourceStorage]).String()
	}

	return ""
}

func (s DiskService) GetStorageProfile(ctx context.Context, name string) (*cdiv1.StorageProfile, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: name}, s.client, &cdiv1.StorageProfile{})
}

type StorageCapabilities struct {
	AccessMode corev1.PersistentVolumeAccessMode
	VolumeMode corev1.PersistentVolumeMode
}

func (cp StorageCapabilities) IsEmpty() bool {
	return cp.AccessMode == "" && cp.VolumeMode == ""
}

var accessModeWeights = map[corev1.PersistentVolumeAccessMode]int{
	corev1.ReadOnlyMany:     0,
	corev1.ReadWriteOncePod: 1,
	corev1.ReadWriteOnce:    2,
	corev1.ReadWriteMany:    3,
}

var volumeModeWeights = map[corev1.PersistentVolumeMode]int{
	corev1.PersistentVolumeFilesystem: 0,
	corev1.PersistentVolumeBlock:      1,
}

func getAccessModeMax(modes []corev1.PersistentVolumeAccessMode) corev1.PersistentVolumeAccessMode {
	weight := -1
	var m corev1.PersistentVolumeAccessMode
	for _, mode := range modes {
		if accessModeWeights[mode] > weight {
			weight = accessModeWeights[mode]
			m = mode
		}
	}
	return m
}

func (s DiskService) parseVolumeMode(obj client.Object) (corev1.PersistentVolumeMode, bool) {
	if obj == nil {
		return "", false
	}
	switch obj.GetAnnotations()[annotations.AnnVirtualDiskVolumeMode] {
	case string(corev1.PersistentVolumeBlock):
		return corev1.PersistentVolumeBlock, true
	case string(corev1.PersistentVolumeFilesystem):
		return corev1.PersistentVolumeFilesystem, true
	default:
		return "", false
	}
}

func (s DiskService) parseAccessMode(obj client.Object) (corev1.PersistentVolumeAccessMode, bool) {
	if obj == nil {
		return "", false
	}
	switch obj.GetAnnotations()[annotations.AnnVirtualDiskAccessMode] {
	case string(corev1.ReadWriteOnce):
		return corev1.ReadWriteOnce, true
	case string(corev1.ReadWriteMany):
		return corev1.ReadWriteMany, true
	default:
		return "", false
	}
}

func (s DiskService) isImmediateBindingMode(sc *storagev1.StorageClass) bool {
	if sc == nil {
		return false
	}
	return sc.GetAnnotations()[annotations.AnnVirtualDiskBindingMode] == string(storagev1.VolumeBindingImmediate)
}

func (s DiskService) parseStorageCapabilities(status cdiv1.StorageProfileStatus) StorageCapabilities {
	var storageCapabilities []StorageCapabilities
	for _, cp := range status.ClaimPropertySets {
		var mode corev1.PersistentVolumeMode
		if cp.VolumeMode == nil || *cp.VolumeMode == "" {
			mode = corev1.PersistentVolumeFilesystem
		} else {
			mode = *cp.VolumeMode
		}
		storageCapabilities = append(storageCapabilities, StorageCapabilities{
			AccessMode: getAccessModeMax(cp.AccessModes),
			VolumeMode: mode,
		})
	}
	slices.SortFunc(storageCapabilities, func(a, b StorageCapabilities) int {
		if c := cmp.Compare(accessModeWeights[a.AccessMode], accessModeWeights[b.AccessMode]); c != 0 {
			return c
		}
		return cmp.Compare(volumeModeWeights[a.VolumeMode], volumeModeWeights[b.VolumeMode])
	})

	if len(storageCapabilities) == 0 {
		return StorageCapabilities{
			AccessMode: corev1.ReadWriteOnce,
			VolumeMode: corev1.PersistentVolumeFilesystem,
		}
	}

	return storageCapabilities[len(storageCapabilities)-1]
}

func (s DiskService) GetStorageClass(ctx context.Context, scName string) (*storagev1.StorageClass, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: scName}, s.client, &storagev1.StorageClass{})
}

func (s DiskService) GetDataVolume(ctx context.Context, sup supplements.Generator) (*cdiv1.DataVolume, error) {
	return supplements.FetchSupplement(ctx, s.client, sup, supplements.SupplementDataVolume, &cdiv1.DataVolume{})
}

func (s DiskService) GetPersistentVolumeClaim(ctx context.Context, sup supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
	return supplements.FetchSupplement(ctx, s.client, sup, supplements.SupplementPVC, &corev1.PersistentVolumeClaim{})
}

func (s DiskService) GetVolumeSnapshot(ctx context.Context, name, namespace string) (*vsv1.VolumeSnapshot, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: name, Namespace: namespace}, s.client, &vsv1.VolumeSnapshot{})
}

func (s DiskService) GetVirtualImage(ctx context.Context, name, namespace string) (*v1alpha2.VirtualImage, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: name, Namespace: namespace}, s.client, &v1alpha2.VirtualImage{})
}

func (s DiskService) GetClusterVirtualImage(ctx context.Context, name string) (*v1alpha2.ClusterVirtualImage, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: name}, s.client, &v1alpha2.ClusterVirtualImage{})
}

func (s DiskService) ListVirtualDiskSnapshots(ctx context.Context, namespace string) ([]v1alpha2.VirtualDiskSnapshot, error) {
	var vdSnapshots v1alpha2.VirtualDiskSnapshotList
	err := s.client.List(ctx, &vdSnapshots, &client.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		return nil, err
	}

	return vdSnapshots.Items, nil
}

func (s DiskService) GetVirtualDiskSnapshot(ctx context.Context, name, namespace string) (*v1alpha2.VirtualDiskSnapshot, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: name, Namespace: namespace}, s.client, &v1alpha2.VirtualDiskSnapshot{})
}

func (s DiskService) CheckImportProcess(ctx context.Context, dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim) error {
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

	cdiImporterPrime, err := object.FetchObject(ctx, key, s.client, &corev1.Pod{})
	if err != nil {
		return err
	}

	if cdiImporterPrime != nil {
		podInitializedCond, ok := conditions.GetPodCondition(corev1.PodInitialized, cdiImporterPrime.Status.Conditions)
		if ok && podInitializedCond.Status == corev1.ConditionFalse && strings.Contains(podInitializedCond.Reason, "Error") {
			return fmt.Errorf("%w; %s error %s: %s", ErrDataVolumeNotRunning, key.String(), podInitializedCond.Reason, podInitializedCond.Message)
		}

		podScheduledCond, ok := conditions.GetPodCondition(corev1.PodScheduled, cdiImporterPrime.Status.Conditions)
		if ok && podScheduledCond.Status == corev1.ConditionFalse && strings.Contains(podScheduledCond.Reason, "Error") {
			return fmt.Errorf("%w; %s error %s: %s", ErrDataVolumeNotRunning, key.String(), podScheduledCond.Reason, podScheduledCond.Message)
		}
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
