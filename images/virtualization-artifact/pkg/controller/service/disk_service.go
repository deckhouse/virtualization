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
	"sort"
	"strconv"
	"strings"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	storev1 "k8s.io/api/storage/v1"
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
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
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
	storageClass *string,
	source *cdiv1.DataVolumeSource,
	obj ObjectKind,
	sup *supplements.Generator,
	opts ...Option,
) error {
	dvBuilder := kvbuilder.NewDV(sup.DataVolume())
	dvBuilder.SetDataSource(source)
	dvBuilder.SetOwnerRef(obj, obj.GroupVersionKind())

	for _, opt := range opts {
		switch v := opt.(type) {
		case *NodePlacementOption:
			err := dvBuilder.SetNodePlacement(v.nodePlacement)
			if err != nil {
				return fmt.Errorf("set node placement: %w", err)
			}
		default:
			return fmt.Errorf("unknown Start option")
		}
	}

	sc, err := s.GetStorageClass(ctx, storageClass)
	if err != nil {
		return fmt.Errorf("get storage class: %w", err)
	}

	volumeMode, accessMode, err := s.GetVolumeAndAccessModes(ctx, sc)
	if err != nil {
		return fmt.Errorf("get volume and access modes: %w", err)
	}

	dvBuilder.SetPVC(storageClass, pvcSize, accessMode, volumeMode)

	if s.isImmediateBindingMode(sc) {
		dvBuilder.SetImmediate()
	}

	dv := dvBuilder.GetResource()
	err = s.client.Create(ctx, dv)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	err = networkpolicy.CreateNetworkPolicy(ctx, s.client, dv, s.protection.GetFinalizer())
	if err != nil {
		return fmt.Errorf("failed to create NetworkPolicy: %w", err)
	}

	if source.Blank != nil || source.PVC != nil {
		return nil
	}

	return supplements.EnsureForDataVolume(ctx, s.client, sup, dvBuilder.GetResource(), s.dvcrSettings)
}

func (s DiskService) GetVolumeAndAccessModes(ctx context.Context, sc *storev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error) {
	if sc == nil {
		return "", "", errors.New("storage class is nil")
	}

	var accessMode corev1.PersistentVolumeAccessMode
	var volumeMode corev1.PersistentVolumeMode

	storageProfile, err := s.GetStorageProfile(ctx, sc.Name)
	if err != nil {
		return "", "", fmt.Errorf("get storage profile: %w", err)
	}

	if storageProfile == nil {
		return "", "", fmt.Errorf("storage profile %q not found: %w", sc.Name, ErrStorageProfileNotFound)
	}

	storageCaps := s.parseStorageCapabilities(storageProfile.Status)
	accessMode = storageCaps.AccessMode
	volumeMode = storageCaps.VolumeMode

	if m, override := s.parseVolumeMode(sc); override {
		volumeMode = m
	}
	if m, override := s.parseAccessMode(sc); override {
		accessMode = m
	}

	return volumeMode, accessMode, nil
}

func (s DiskService) StartImmediate(
	ctx context.Context,
	pvcSize resource.Quantity,
	storageClass *string,
	source *cdiv1.DataVolumeSource,
	obj ObjectKind,
	sup *supplements.Generator,
) error {
	sc, err := s.GetStorageClass(ctx, storageClass)
	if err != nil {
		return err
	}

	dvBuilder := kvbuilder.NewDV(sup.DataVolume())
	dvBuilder.SetDataSource(source)
	dvBuilder.SetOwnerRef(obj, obj.GroupVersionKind())
	dvBuilder.SetPVC(ptr.To(sc.GetName()), pvcSize, corev1.ReadWriteMany, corev1.PersistentVolumeBlock)
	dvBuilder.SetImmediate()
	dv := dvBuilder.GetResource()

	err = s.client.Create(ctx, dv)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	err = networkpolicy.CreateNetworkPolicy(ctx, s.client, dv, s.protection.GetFinalizer())
	if err != nil {
		return fmt.Errorf("failed to create NetworkPolicy: %w", err)
	}

	if source.PVC != nil {
		return nil
	}

	return supplements.EnsureForDataVolume(ctx, s.client, sup, dvBuilder.GetResource(), s.dvcrSettings)
}

func (s DiskService) CheckProvisioning(ctx context.Context, pvc *corev1.PersistentVolumeClaim) error {
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

		networkPolicy, err := networkpolicy.GetNetworkPolicy(ctx, s.client, sup.DataVolume())
		if err != nil {
			return false, err
		}

		if networkPolicy != nil {
			err = s.protection.RemoveProtection(ctx, networkPolicy)
			if err != nil {
				return false, err
			}

			err = s.client.Delete(ctx, networkPolicy)
			if err != nil && !k8serrors.IsNotFound(err) {
				return false, err
			}
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

	if dv != nil {
		networkPolicy, err := networkpolicy.GetNetworkPolicy(ctx, s.client, types.NamespacedName{Namespace: dv.Namespace, Name: dv.Name})
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

func (s DiskService) Unprotect(ctx context.Context, dv *cdiv1.DataVolume) error {
	err := s.protection.RemoveProtection(ctx, dv)
	if err != nil {
		return fmt.Errorf("failed to remove protection for disk's supplements: %w", err)
	}

	if dv != nil {
		networkPolicy, err := networkpolicy.GetNetworkPolicy(ctx, s.client, types.NamespacedName{Namespace: dv.Namespace, Name: dv.Name})
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

func (s DiskService) parseVolumeMode(sc *storev1.StorageClass) (corev1.PersistentVolumeMode, bool) {
	if sc == nil {
		return "", false
	}
	switch sc.GetAnnotations()[annotations.AnnVirtualDiskVolumeMode] {
	case string(corev1.PersistentVolumeBlock):
		return corev1.PersistentVolumeBlock, true
	case string(corev1.PersistentVolumeFilesystem):
		return corev1.PersistentVolumeFilesystem, true
	default:
		return "", false
	}
}

func (s DiskService) parseAccessMode(sc *storev1.StorageClass) (corev1.PersistentVolumeAccessMode, bool) {
	if sc == nil {
		return "", false
	}
	switch sc.GetAnnotations()[annotations.AnnVirtualDiskAccessMode] {
	case string(corev1.ReadWriteOnce):
		return corev1.ReadWriteOnce, true
	case string(corev1.ReadWriteMany):
		return corev1.ReadWriteMany, true
	default:
		return "", false
	}
}

func (s DiskService) isImmediateBindingMode(sc *storev1.StorageClass) bool {
	if sc == nil {
		return false
	}
	return sc.GetAnnotations()[annotations.AnnVirtualDiskBindingMode] == string(storev1.VolumeBindingImmediate)
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

func (s DiskService) GetDataVolume(ctx context.Context, sup *supplements.Generator) (*cdiv1.DataVolume, error) {
	return object.FetchObject(ctx, sup.DataVolume(), s.client, &cdiv1.DataVolume{})
}

func (s DiskService) GetPersistentVolumeClaim(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
	return object.FetchObject(ctx, sup.PersistentVolumeClaim(), s.client, &corev1.PersistentVolumeClaim{})
}

func (s DiskService) GetVolumeSnapshot(ctx context.Context, name, namespace string) (*vsv1.VolumeSnapshot, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: name, Namespace: namespace}, s.client, &vsv1.VolumeSnapshot{})
}

func (s DiskService) GetVirtualImage(ctx context.Context, name, namespace string) (*virtv2.VirtualImage, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: name, Namespace: namespace}, s.client, &virtv2.VirtualImage{})
}

func (s DiskService) GetClusterVirtualImage(ctx context.Context, name string) (*virtv2.ClusterVirtualImage, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: name}, s.client, &virtv2.ClusterVirtualImage{})
}

func (s DiskService) ListVirtualDiskSnapshots(ctx context.Context, namespace string) ([]virtv2.VirtualDiskSnapshot, error) {
	var vdSnapshots virtv2.VirtualDiskSnapshotList
	err := s.client.List(ctx, &vdSnapshots, &client.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		return nil, err
	}

	return vdSnapshots.Items, nil
}

func (s DiskService) GetVirtualDiskSnapshot(ctx context.Context, name, namespace string) (*virtv2.VirtualDiskSnapshot, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: name, Namespace: namespace}, s.client, &virtv2.VirtualDiskSnapshot{})
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

func (s DiskService) GetStorageClass(ctx context.Context, storageClassName *string) (*storev1.StorageClass, error) {
	if storageClassName == nil || *storageClassName == "" {
		return s.GetDefaultStorageClass(ctx)
	}
	return s.getStorageClass(ctx, *storageClassName)
}

func (s DiskService) GetDefaultStorageClass(ctx context.Context) (*storev1.StorageClass, error) {
	var (
		moduleConfigViDefaultStorageClass string
		moduleConfig                      mcapi.ModuleConfig
		moduleConfigName                  = "virtualization"
	)
	err := s.client.Get(ctx, types.NamespacedName{Name: moduleConfigName}, &moduleConfig, &client.GetOptions{})
	if err != nil {
		return nil, err
	}

	if virtualImages, ok := moduleConfig.Spec.Settings["virtualImages"].(map[string]interface{}); ok {
		if defaultClass, ok := virtualImages["defaultStorageClassName"].(string); ok {
			moduleConfigViDefaultStorageClass = defaultClass
		}
	}

	if moduleConfigViDefaultStorageClass != "" {
		moduleConfigViDefaultStorageClassObj, err := s.getStorageClass(ctx, moduleConfigViDefaultStorageClass)
		if err != nil {
			return nil, err
		}
		return moduleConfigViDefaultStorageClassObj, nil
	}

	var scs storev1.StorageClassList
	err = s.client.List(ctx, &scs, &client.ListOptions{})
	if err != nil {
		return nil, err
	}

	var defaultClasses []*storev1.StorageClass
	for idx := range scs.Items {
		if scs.Items[idx].Annotations[annotations.AnnDefaultStorageClass] == "true" {
			defaultClasses = append(defaultClasses, &scs.Items[idx])
		}
	}

	if len(defaultClasses) == 0 {
		return nil, ErrDefaultStorageClassNotFound
	}

	// Primary sort by creation timestamp, newest first
	// Secondary sort by class name, ascending order
	sort.Slice(defaultClasses, func(i, j int) bool {
		if defaultClasses[i].CreationTimestamp.UnixNano() == defaultClasses[j].CreationTimestamp.UnixNano() {
			return defaultClasses[i].Name < defaultClasses[j].Name
		}
		return defaultClasses[i].CreationTimestamp.UnixNano() > defaultClasses[j].CreationTimestamp.UnixNano()
	})

	return defaultClasses[0], nil
}

func (s DiskService) getStorageClass(ctx context.Context, storageClassName string) (*storev1.StorageClass, error) {
	var sc storev1.StorageClass
	err := s.client.Get(ctx, types.NamespacedName{
		Name: storageClassName,
	}, &sc, &client.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, ErrStorageClassNotFound
		}

		return nil, err
	}

	return &sc, nil
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
