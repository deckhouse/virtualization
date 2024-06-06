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
	"log/slog"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/deckhouse/virtualization-controller/pkg/common"
	vdutil "github.com/deckhouse/virtualization-controller/pkg/common/datavolume"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type ObjectRefDataSource struct {
	statService *service.StatService
	diskService *service.DiskService
	client      client.Client
	logger      *slog.Logger
}

func NewObjectRefDataSource(
	statService *service.StatService,
	diskService *service.DiskService,
	client client.Client,
) *ObjectRefDataSource {
	return &ObjectRefDataSource{
		statService: statService,
		diskService: diskService,
		client:      client,
		logger:      slog.Default().With("controller", common.VDShortName, "ds", "objectref"),
	}
}

func (ds ObjectRefDataSource) Sync(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	ds.logger.Info("Sync", "vd", vd.Name)

	condition, _ := service.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
	defer func() { service.SetCondition(condition, &vd.Status.Conditions) }()

	supgen := supplements.NewGenerator(common.VDShortName, vd.Name, vd.Namespace, vd.UID)
	dv, err := ds.diskService.GetDataVolume(ctx, supgen)
	if err != nil {
		return false, err
	}
	pvc, err := ds.diskService.GetPersistentVolumeClaim(ctx, supgen)
	if err != nil {
		return false, err
	}
	pv, err := ds.diskService.GetPersistentVolume(ctx, pvc)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		ds.logger.Info("Finishing...", "vd", vd.Name)

		switch {
		case pvc == nil:
			condition.Status = metav1.ConditionFalse
			condition.Reason = vdcondition.ReadyReason_Lost
			condition.Message = fmt.Sprintf("PVC %s not found.", supgen.PersistentVolumeClaim().String())
		case pv == nil:
			condition.Status = metav1.ConditionFalse
			condition.Reason = vdcondition.ReadyReason_Lost
			condition.Message = fmt.Sprintf("PV %s not found.", pvc.Spec.VolumeName)
		default:
			condition.Status = metav1.ConditionTrue
			condition.Reason = vdcondition.ReadyReason_Ready
			condition.Message = ""
		}

		// Protect Ready Disk and underlying PVC and PV.
		err = ds.diskService.Protect(ctx, vd, nil, pvc, pv)
		if err != nil {
			return false, err
		}

		err = ds.diskService.Unprotect(ctx, dv)
		if err != nil {
			return false, err
		}

		return CleanUpSupplements(ctx, vd, ds)
	case common.AnyTerminating(dv, pvc, pv):
		vd.Status.Phase = virtv2.DiskPending

		ds.logger.Info("Cleaning up...", "vd", vd.Name)
	case dv == nil:
		var diskSize resource.Quantity
		diskSize, err = ds.getPVCSize(ctx, vd)
		if err != nil {
			return false, err
		}

		var source *cdiv1.DataVolumeSource
		source, err = ds.getSource(ctx, vd, supgen)
		if err != nil {
			return false, err
		}

		err = ds.diskService.Start(ctx, diskSize, vd.Spec.PersistentVolumeClaim.StorageClass, source, vd, supgen)
		if err != nil {
			return false, err
		}

		vd.Status.Phase = virtv2.DiskProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = vdcondition.ReadyReason_Provisioning
		condition.Message = "PVC Provisioner not found: create the new one."

		vd.Status.Progress = "0%"

		ds.logger.Info("Create data volume...", "vd", vd.Name, "progress", vd.Status.Progress, "dv.phase", "nil")

		return true, nil
	case common.IsDataVolumeComplete(dv):
		vd.Status.Phase = virtv2.DiskReady
		condition.Status = metav1.ConditionTrue
		condition.Reason = vdcondition.ReadyReason_Ready
		condition.Message = ""

		vd.Status.Progress = "100%"
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		ds.logger.Info("Ready", "vd", vd.Name, "progress", vd.Status.Progress, "dv.phase", dv.Status.Phase)
	default:
		ds.logger.Info("Provisioning...", "vd", vd.Name, "progress", vd.Status.Progress, "dv.phase", dv.Status.Phase)

		vd.Status.Progress = ds.diskService.GetProgress(dv, vd.Status.Progress, service.NewScaleOption(0, 100))
		vd.Status.Capacity = ds.diskService.GetCapacity(pvc)
		vd.Status.Target.PersistentVolumeClaim = dv.Status.ClaimName

		err = ds.diskService.Protect(ctx, vd, dv, pvc, pv)
		if err != nil {
			return false, err
		}

		err = ds.diskService.CheckStorageClass(ctx, vd.Spec.PersistentVolumeClaim.StorageClass)
		switch {
		case err == nil:
			vd.Status.Phase = virtv2.DiskProvisioning
			condition.Status = metav1.ConditionFalse
			condition.Reason = vdcondition.ReadyReason_Provisioning
			condition.Message = "Import is in the process of provisioning to PVC."
			return false, nil
		case errors.Is(err, service.ErrStorageClassNotFound):
			vd.Status.Phase = virtv2.DiskFailed
			condition.Status = metav1.ConditionFalse
			condition.Reason = vdcondition.ReadyReason_ProvisioningFailed
			condition.Message = "Provided StorageClass not found in the cluster."
			return false, nil
		case errors.Is(err, service.ErrDefaultStorageClassNotFound):
			vd.Status.Phase = virtv2.DiskFailed
			condition.Status = metav1.ConditionFalse
			condition.Reason = vdcondition.ReadyReason_ProvisioningFailed
			condition.Message = "Default StorageClass not found in the cluster: please provide a StorageClass name or set a default StorageClass."
			return false, nil
		default:
			return false, err
		}
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

func (ds ObjectRefDataSource) getSource(ctx context.Context, vd *virtv2.VirtualDisk, sup *supplements.Generator) (*cdiv1.DataVolumeSource, error) {
	dvcrDataSource, err := controller.NewDVCRDataSourcesForVMD(ctx, vd.Spec.DataSource, vd, ds.client)
	if err != nil {
		return nil, err
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

func (ds ObjectRefDataSource) getPVCSize(ctx context.Context, vd *virtv2.VirtualDisk) (resource.Quantity, error) {
	dvcrDataSource, err := controller.NewDVCRDataSourcesForVMD(ctx, vd.Spec.DataSource, vd, ds.client)
	if err != nil {
		return resource.Quantity{}, err
	}

	unpackedSize, err := resource.ParseQuantity(dvcrDataSource.GetSize().UnpackedBytes)
	if err != nil {
		return resource.Quantity{}, err
	}

	if unpackedSize.IsZero() {
		return resource.Quantity{}, errors.New("got zero unpacked size from data source")
	}

	pvcSize := vd.Spec.PersistentVolumeClaim.Size
	if pvcSize != nil && !pvcSize.IsZero() && pvcSize.Cmp(unpackedSize) == -1 {
		return resource.Quantity{}, ErrPVCSizeSmallerImageVirtualSize
	}

	// Adjust PVC size to feat image onto scratch PVC.
	// TODO(future): remove size adjusting after get rid of scratch.
	adjustedSize := vdutil.AdjustPVCSize(unpackedSize)

	if pvcSize != nil && pvcSize.Cmp(adjustedSize) == 1 {
		return *pvcSize, nil
	}

	return adjustedSize, nil
}