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

package source

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	"github.com/deckhouse/virtualization-controller/pkg/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

const objectRefDataSource = "objectref"

type ObjectRefDataSource struct {
	statService *service.StatService
	diskService *service.DiskService
	client      client.Client

	vdSnapshotSyncer *ObjectRefVirtualDiskSnapshot
	viOnPvcSyncer    *ObjectRefVirtualImageOnPvc
}

func NewObjectRefDataSource(
	statService *service.StatService,
	diskService *service.DiskService,
	client client.Client,
) *ObjectRefDataSource {
	return &ObjectRefDataSource{
		statService:      statService,
		diskService:      diskService,
		client:           client,
		vdSnapshotSyncer: NewObjectRefVirtualDiskSnapshot(diskService),
		viOnPvcSyncer:    NewObjectRefVirtualImageOnPvc(diskService),
	}
}

func (ds ObjectRefDataSource) Sync(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	log, ctx := logger.GetDataSourceContext(ctx, objectRefDataSource)

	condition, _ := service.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	defer func() { service.SetCondition(condition, &vd.Status.Conditions) }()

	switch vd.Spec.DataSource.ObjectRef.Kind {
	case virtv2.VirtualDiskObjectRefKindVirtualDiskSnapshot:
		return ds.vdSnapshotSyncer.Sync(ctx, vd, &condition)
	case virtv2.VirtualImageKind:
		viKey := types.NamespacedName{Name: vd.Spec.DataSource.ObjectRef.Name, Namespace: vd.Namespace}
		vi, err := helper.FetchObject(ctx, viKey, ds.client, &virtv2.VirtualImage{})
		if err != nil {
			return false, fmt.Errorf("unable to get VI %s: %w", viKey, err)
		}

		if vi == nil {
			return false, fmt.Errorf("VI object ref source %s is nil", vd.Spec.DataSource.ObjectRef.Name)
		}

		if vi.Spec.Storage == virtv2.StorageKubernetes {
			return ds.viOnPvcSyncer.Sync(ctx, vd, vi, &condition)
		}
	}

	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)
	dv, err := ds.diskService.GetDataVolume(ctx, supgen)
	if err != nil {
		return false, err
	}
	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Disk provisioning finished: clean up")

		setPhaseConditionForFinishedDisk(pvc, &condition, &vd.Status.Phase, supgen)

		// Protect Ready Disk and underlying PVC.
		err = ds.diskService.Protect(ctx, vd, nil, pvc)
		if err != nil {
			return false, err
		}

		err = ds.diskService.Unprotect(ctx, dv)
		if err != nil {
			return false, err
		}

		return CleanUpSupplements(ctx, vd, ds)
	case common.AnyTerminating(dv, pvc):
		log.Info("Waiting for supplements to be terminated")
	case dv == nil:
		log.Info("Start import to PVC")

		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForVMD(ctx, vd.Spec.DataSource, vd, ds.client)
		if err != nil {
			return false, err
		}

		if !dvcrDataSource.IsReady() {
			condition.Status = metav1.ConditionFalse
			condition.Reason = vdcondition.ProvisioningFailed
			condition.Message = "Failed to get stats from non-ready datasource: waiting for the DataSource to be ready."
			return false, nil
		}

		vd.Status.Progress = "0%"
		vd.Status.SourceUID = util.GetPointer(dvcrDataSource.GetUID())

		if imageformat.IsISO(dvcrDataSource.GetFormat()) {
			setPhaseConditionToFailed(&condition, &vd.Status.Phase, ErrISOSourceNotSupported)
			return false, nil
		}

		var diskSize resource.Quantity
		diskSize, err = ds.getPVCSize(vd, dvcrDataSource)
		if err != nil {
			setPhaseConditionToFailed(&condition, &vd.Status.Phase, err)

			if errors.Is(err, service.ErrInsufficientPVCSize) {
				return false, nil
			}

			return false, err
		}

		var source *cdiv1.DataVolumeSource
		source, err = ds.getSource(supgen, dvcrDataSource)
		if err != nil {
			return false, err
		}

		err = ds.diskService.Start(ctx, diskSize, vd.Spec.PersistentVolumeClaim.StorageClass, source, vd, supgen)
		if updated, err := setPhaseConditionFromStorageError(err, vd, &condition); err != nil || updated {
			return false, err
		}

		vd.Status.Phase = virtv2.DiskProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.Provisioning
		condition.Message = "PVC Provisioner not found: create the new one."

		return true, nil
	case pvc == nil:
		vd.Status.Phase = virtv2.DiskProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.Provisioning
		condition.Message = "PVC not found: waiting for creation."
		return true, nil
	case ds.diskService.IsImportDone(dv, pvc):
		log.Info("Import has completed", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vd.Status.Phase = virtv2.DiskReady
		condition.Status = metav1.ConditionTrue
		condition.Reason = vdcondition.Ready
		condition.Message = ""

		vd.Status.Progress = "100%"
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName
	default:
		log.Info("Provisioning to PVC is in progress", "dvProgress", dv.Status.Progress, "dvPhase", dv.Status.Phase, "pvcPhase", pvc.Status.Phase)

		vd.Status.Progress = ds.diskService.GetProgress(dv, vd.Status.Progress, service.NewScaleOption(0, 100))
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		err = ds.diskService.Protect(ctx, vd, dv, pvc)
		if err != nil {
			return false, err
		}
		sc, err := ds.diskService.GetStorageClass(ctx, pvc.Spec.StorageClassName)
		if updated, err := setPhaseConditionFromStorageError(err, vd, &condition); err != nil || updated {
			return false, err
		}
		if err = setPhaseConditionForPVCProvisioningDisk(ctx, dv, vd, pvc, sc, &condition, ds.diskService); err != nil {
			return false, err
		}

		return false, nil
	}

	return true, nil
}

func (ds ObjectRefDataSource) CleanUp(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)

	requeue, err := ds.diskService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	return requeue, nil
}

func (ds ObjectRefDataSource) CleanUpSupplements(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)

	requeue, err := ds.diskService.CleanUpSupplements(ctx, supgen)
	if err != nil {
		return false, err
	}

	return requeue, nil
}

func (ds ObjectRefDataSource) Validate(ctx context.Context, vd *virtv2.VirtualDisk) error {
	if vd.Spec.DataSource == nil || vd.Spec.DataSource.ObjectRef == nil {
		return errors.New("object ref missed for data source")
	}

	if vd.Spec.DataSource.ObjectRef.Kind == virtv2.VirtualDiskObjectRefKindVirtualDiskSnapshot {
		return ds.vdSnapshotSyncer.Validate(ctx, vd)
	}

	if vd.Spec.DataSource.ObjectRef.Kind == virtv2.VirtualImageKind {
		viKey := types.NamespacedName{Name: vd.Spec.DataSource.ObjectRef.Name, Namespace: vd.Namespace}
		vi, err := helper.FetchObject(ctx, viKey, ds.client, &virtv2.VirtualImage{})
		if err != nil {
			return fmt.Errorf("unable to get VI %s: %w", viKey, err)
		}

		if vi == nil {
			return NewImageNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
		}

		if vi.Spec.Storage == virtv2.StorageKubernetes {
			if vi.Status.Phase != virtv2.ImageReady {
				return NewImageNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
			}
			return nil
		}
	}

	dvcrDataSource, err := controller.NewDVCRDataSourcesForVMD(ctx, vd.Spec.DataSource, vd, ds.client)
	if err != nil {
		return err
	}

	if dvcrDataSource.IsReady() {
		return nil
	}

	switch vd.Spec.DataSource.ObjectRef.Kind {
	case virtv2.VirtualDiskObjectRefKindVirtualImage:
		return NewImageNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
	case virtv2.VirtualDiskObjectRefKindClusterVirtualImage:
		return NewClusterImageNotReadyError(vd.Spec.DataSource.ObjectRef.Name)
	default:
		return fmt.Errorf("unexpected object ref kind: %s", vd.Spec.DataSource.ObjectRef.Kind)
	}
}

func (ds ObjectRefDataSource) Name() string {
	return objectRefDataSource
}

func (ds ObjectRefDataSource) getSource(sup *supplements.Generator, dvcrDataSource controller.DVCRDataSource) (*cdiv1.DataVolumeSource, error) {
	if !dvcrDataSource.IsReady() {
		return nil, errors.New("dvcr data source is not ready")
	}

	url := common2.DockerRegistrySchemePrefix + dvcrDataSource.GetTarget()
	secretName := sup.DVCRAuthSecretForDV().Name
	certConfigMapName := sup.DVCRCABundleConfigMapForDV().Name

	return &cdiv1.DataVolumeSource{
		Registry: &cdiv1.DataVolumeSourceRegistry{
			URL:           &url,
			SecretRef:     &secretName,
			CertConfigMap: &certConfigMapName,
		},
	}, nil
}

func (ds ObjectRefDataSource) getPVCSize(vd *virtv2.VirtualDisk, dvcrDataSource controller.DVCRDataSource) (resource.Quantity, error) {
	if !dvcrDataSource.IsReady() {
		return resource.Quantity{}, errors.New("dvcr data source is not ready")
	}

	unpackedSize, err := resource.ParseQuantity(dvcrDataSource.GetSize().UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to parse unpacked bytes %s: %w", dvcrDataSource.GetSize().UnpackedBytes, err)
	}

	return service.GetValidatedPVCSize(vd.Spec.PersistentVolumeClaim.Size, unpackedSize)
}
