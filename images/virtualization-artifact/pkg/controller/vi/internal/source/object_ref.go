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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ObjectRefDataSource struct {
	statService     Stat
	importerService Importer
	dvcrSettings    *dvcr.Settings
	client          client.Client
	logger          *slog.Logger
}

func NewObjectRefDataSource(
	statService Stat,
	importerService Importer,
	dvcrSettings *dvcr.Settings,
	client client.Client,
) *ObjectRefDataSource {
	return &ObjectRefDataSource{
		statService:     statService,
		importerService: importerService,
		dvcrSettings:    dvcrSettings,
		client:          client,
		logger:          slog.Default().With("controller", cc.VIShortName, "ds", "objectref"),
	}
}

func (ds ObjectRefDataSource) Sync(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	ds.logger.Info("Sync", "vi", vi.Name)

	condition, _ := service.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
	defer func() { service.SetCondition(condition, &vi.Status.Conditions) }()

	supgen := supplements.NewGenerator(cc.VIShortName, vi.Name, vi.Namespace, vi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		ds.logger.Info("Finishing...", "vi", vi.Name)

		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		vi.Status.Phase = virtv2.ImageReady

		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return false, err
		}

		return CleanUp(ctx, vi, ds)
	case cc.IsTerminating(pod):
		vi.Status.Phase = virtv2.ImagePending

		ds.logger.Info("Cleaning up...", "vi", vi.Name)
	case pod == nil:
		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "DVCR Provisioner not found: create the new one."

		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
		if err != nil {
			return false, err
		}

		var envSettings *importer.Settings
		envSettings, err = ds.getEnvSettings(vi, supgen, dvcrDataSource)
		if err != nil {
			return false, err
		}

		err = ds.importerService.Start(ctx, envSettings, vi, supgen, datasource.NewCABundleForVMI(vi.Spec.DataSource))
		if err != nil {
			return false, err
		}

		vi.Status.Phase = virtv2.ImageProvisioning
		vi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForVMI(vi.Name, vi.Namespace)
		vi.Status.SourceUID = util.GetPointer(dvcrDataSource.GetUID())

		ds.logger.Info("Ready", "vi", vi.Name, "progress", vi.Status.Progress, "pod.phase", "nil")
	case cc.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			default:
				return false, err
			}
		}

		var dvcrDataSource controller.DVCRDataSource
		dvcrDataSource, err = controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
		if err != nil {
			return false, err
		}

		if !dvcrDataSource.IsReady() {
			condition.Status = metav1.ConditionFalse
			condition.Reason = vicondition.ProvisioningFailed
			condition.Message = "Failed to get stats from non-ready datasource: waiting for the DataSource to be ready."
			return false, nil
		}

		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		vi.Status.Phase = virtv2.ImageReady
		vi.Status.Size = dvcrDataSource.GetSize()
		vi.Status.CDROM = dvcrDataSource.IsCDROM()
		vi.Status.Format = dvcrDataSource.GetFormat()
		vi.Status.Progress = "100%"
		vi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForVMI(vi.Name, vi.Namespace)

		ds.logger.Info("Ready", "vi", vi.Name, "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	default:
		err = ds.statService.CheckPod(pod)
		if err != nil {
			vi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vicondition.ProvisioningNotStarted
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			default:
				return false, err
			}
		}

		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "Import is in the process of provisioning to DVCR."

		vi.Status.Phase = virtv2.ImageProvisioning
		vi.Status.Target.RegistryURL = ds.dvcrSettings.RegistryImageForVMI(vi.Name, vi.Namespace)

		ds.logger.Info("Ready", "vi", vi.Name, "progress", vi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return true, nil
}

func (ds ObjectRefDataSource) CleanUp(ctx context.Context, vi *virtv2.VirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(cc.VIShortName, vi.Name, vi.Namespace, vi.UID)

	requeue, err := ds.importerService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	return requeue, nil
}

func (ds ObjectRefDataSource) Validate(ctx context.Context, vi *virtv2.VirtualImage) error {
	if vi.Spec.DataSource.ObjectRef == nil {
		return fmt.Errorf("nil object ref: %s", vi.Spec.DataSource.Type)
	}

	dvcrDataSource, err := controller.NewDVCRDataSourcesForVMI(ctx, vi.Spec.DataSource, vi, ds.client)
	if err != nil {
		return err
	}

	if dvcrDataSource.IsReady() {
		return nil
	}

	switch vi.Spec.DataSource.ObjectRef.Kind {
	case virtv2.VirtualImageObjectRefKindVirtualImage:
		return NewImageNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
	case virtv2.VirtualImageObjectRefKindClusterVirtualImage:
		return NewClusterImageNotReadyError(vi.Spec.DataSource.ObjectRef.Name)
	default:
		return fmt.Errorf("unexpected object ref kind: %s", vi.Spec.DataSource.ObjectRef.Kind)
	}
}

func (ds ObjectRefDataSource) getEnvSettings(vi *virtv2.VirtualImage, sup *supplements.Generator, dvcrDataSource controller.DVCRDataSource) (*importer.Settings, error) {
	if !dvcrDataSource.IsReady() {
		return nil, errors.New("dvcr data source is not ready")
	}

	var settings importer.Settings
	importer.ApplyDVCRSourceSettings(&settings, dvcrDataSource.GetTarget())
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		sup,
		ds.dvcrSettings.RegistryImageForVMI(vi.Name, vi.Namespace),
	)

	return &settings, nil
}
