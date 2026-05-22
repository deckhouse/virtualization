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
	"strconv"

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
	networkpolicy "github.com/deckhouse/virtualization-controller/pkg/common/network_policy"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/volumemode"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type DiskService struct {
	client               client.Client
	dvcrSettings         *dvcr.Settings
	protection           *ProtectionService
	controllerName       string
	diskImporterImage    string
	resourceRequirements corev1.ResourceRequirements
	pullPolicy           string
	verbose              string

	volumeAndAccessModesGetter volumemode.VolumeAndAccessModesGetter
}

type DiskImporterConfig struct {
	Image                string
	ResourceRequirements corev1.ResourceRequirements
	PullPolicy           string
	Verbose              string
}

func NewDiskService(
	client client.Client,
	dvcrSettings *dvcr.Settings,
	protection *ProtectionService,
	controllerName string,
	diskImporterConfig ...DiskImporterConfig,
) *DiskService {
	var cfg DiskImporterConfig
	var requirements corev1.ResourceRequirements
	if len(diskImporterConfig) > 0 {
		cfg = diskImporterConfig[0]
		requirements = cfg.ResourceRequirements
	}

	return &DiskService{
		client:                     client,
		dvcrSettings:               dvcrSettings,
		protection:                 protection,
		controllerName:             controllerName,
		diskImporterImage:          cfg.Image,
		resourceRequirements:       requirements,
		pullPolicy:                 cfg.PullPolicy,
		verbose:                    cfg.Verbose,
		volumeAndAccessModesGetter: volumemode.NewVolumeAndAccessModesGetter(client, nil),
	}
}

func (s DiskService) GetVolumeAndAccessModes(ctx context.Context, obj client.Object, sc *storagev1.StorageClass) (corev1.PersistentVolumeMode, corev1.PersistentVolumeAccessMode, error) {
	return s.volumeAndAccessModesGetter.GetVolumeAndAccessModes(ctx, obj, sc)
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
		return fmt.Errorf("failed to fetch pvc provisioner %s: %w", podName, err)
	}

	if pod == nil {
		return nil
	}

	scheduled, _ := conditions.GetPodCondition(corev1.PodScheduled, pod.Status.Conditions)
	if scheduled.Status == corev1.ConditionFalse && scheduled.Reason == corev1.PodReasonUnschedulable {
		return ErrProvisionerUnschedulable
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
	target := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sup.PersistentVolumeClaim().Name,
			Namespace: sup.PersistentVolumeClaim().Namespace,
		},
	}
	importSupplementsDeleted, err := s.cleanupPVCImport(ctx, target)
	if err != nil {
		return false, fmt.Errorf("delete pvc import supplements: %w", err)
	}

	networkPolicy, err := networkpolicy.GetNetworkPolicy(ctx, s.client, sup.LegacyCommonResourceName(), sup)
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

	return importSupplementsDeleted || networkPolicy != nil, nil
}

func (s DiskService) Protect(ctx context.Context, _ supplements.Generator, owner client.Object, pvc *corev1.PersistentVolumeClaim) error {
	err := s.protection.AddOwnerRef(ctx, owner, pvc)
	if err != nil {
		return fmt.Errorf("failed to add owner ref for pvc: %w", err)
	}

	err = s.protection.AddProtection(ctx, pvc)
	if err != nil {
		return fmt.Errorf("failed to add protection for disk's supplements: %w", err)
	}

	return nil
}

func (s DiskService) Unprotect(_ context.Context, _ supplements.Generator) error {
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

func (s DiskService) GetCapacity(pvc *corev1.PersistentVolumeClaim) string {
	if pvc != nil && pvc.Status.Phase == corev1.ClaimBound {
		return ptr.To(pvc.Status.Capacity[corev1.ResourceStorage]).String()
	}

	return ""
}

func (s DiskService) GetStorageProfile(ctx context.Context, name string) (*cdiv1.StorageProfile, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: name}, s.client, &cdiv1.StorageProfile{})
}

func (s DiskService) isImmediateBindingMode(sc *storagev1.StorageClass) bool {
	if sc == nil {
		return false
	}
	return sc.GetAnnotations()[annotations.AnnVirtualDiskBindingMode] == string(storagev1.VolumeBindingImmediate)
}

func (s DiskService) GetStorageClass(ctx context.Context, scName string) (*storagev1.StorageClass, error) {
	return object.FetchObject(ctx, types.NamespacedName{Name: scName}, s.client, &storagev1.StorageClass{})
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
