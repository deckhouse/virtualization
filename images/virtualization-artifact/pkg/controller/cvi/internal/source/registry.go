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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type RegistryDataSource struct {
	statService         Stat
	importerService     Importer
	dvcrSettings        *dvcr.Settings
	client              client.Client
	controllerNamespace string
}

func NewRegistryDataSource(
	statService Stat,
	importerService Importer,
	dvcrSettings *dvcr.Settings,
	client client.Client,
	controllerNamespace string,
) *RegistryDataSource {
	return &RegistryDataSource{
		statService:         statService,
		importerService:     importerService,
		dvcrSettings:        dvcrSettings,
		client:              client,
		controllerNamespace: controllerNamespace,
	}
}

func (ds RegistryDataSource) Sync(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (bool, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "registry")

	condition, _ := service.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
	defer func() { service.SetCondition(condition, &cvi.Status.Conditions) }()

	supgen := supplements.NewGenerator(common.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return false, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Cluster virtual image provisioning finished: clean up")

		condition.Status = metav1.ConditionTrue
		condition.Reason = cvicondition.Ready
		condition.Message = ""

		cvi.Status.Phase = virtv2.ImageReady

		// Unprotect import time supplements to delete them later.
		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return false, err
		}

		return CleanUp(ctx, cvi, ds)
	case common.IsTerminating(pod):
		cvi.Status.Phase = virtv2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil:
		cvi.Status.Progress = "0%"

		envSettings := ds.getEnvSettings(cvi, supgen)
		err = ds.importerService.Start(ctx, envSettings, cvi, supgen, datasource.NewCABundleForCVMI(cvi.Spec.DataSource), "")
		var requeue bool
		requeue, err = setPhaseConditionForImporterStart(&condition, &cvi.Status.Phase, err)
		if err != nil {
			return false, err
		}

		log.Info("Create importer pod...", "progress", cvi.Status.Progress, "pod.phase", "nil")

		return requeue, nil
	case common.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			cvi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = cvicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			default:
				return false, err
			}
		}

		condition.Status = metav1.ConditionTrue
		condition.Reason = cvicondition.Ready
		condition.Message = ""

		cvi.Status.Phase = virtv2.ImageReady
		cvi.Status.Size = ds.statService.GetSize(pod)
		cvi.Status.CDROM = ds.statService.GetCDROM(pod)
		cvi.Status.Format = ds.statService.GetFormat(pod)
		cvi.Status.Progress = "100%"
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Ready", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	default:
		err = ds.statService.CheckPod(pod)
		if err != nil {
			cvi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
				condition.Status = metav1.ConditionFalse
				condition.Reason = cvicondition.ProvisioningNotStarted
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = cvicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return false, nil
			default:
				return false, err
			}
		}

		condition.Status = metav1.ConditionFalse
		condition.Reason = cvicondition.Provisioning
		condition.Message = "Import is in the process of provisioning to DVCR."

		cvi.Status.Phase = virtv2.ImageProvisioning
		cvi.Status.Progress = "0%"
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Provisioning...", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return true, nil
}

func (ds RegistryDataSource) CleanUp(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(common.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)

	requeue, err := ds.importerService.CleanUp(ctx, supgen)
	if err != nil {
		return false, err
	}

	return requeue, nil
}

func (ds RegistryDataSource) Validate(ctx context.Context, cvi *virtv2.ClusterVirtualImage) error {
	if cvi.Spec.DataSource.ContainerImage.ImagePullSecret.Name != "" {
		secretName := types.NamespacedName{
			Namespace: cvi.Spec.DataSource.ContainerImage.ImagePullSecret.Namespace,
			Name:      cvi.Spec.DataSource.ContainerImage.ImagePullSecret.Name,
		}
		secret, err := helper.FetchObject[*corev1.Secret](ctx, secretName, ds.client, &corev1.Secret{})
		if err != nil {
			return fmt.Errorf("failed to get secret %s: %w", secretName, err)
		}

		if secret == nil {
			return ErrSecretNotFound
		}
	}

	return nil
}

func (ds RegistryDataSource) getEnvSettings(cvi *virtv2.ClusterVirtualImage, supgen *supplements.Generator) *importer.Settings {
	var settings importer.Settings

	importer.ApplyRegistrySourceSettings(&settings, cvi.Spec.DataSource.ContainerImage, supgen)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForCVI(cvi),
	)

	return &settings
}
