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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type HTTPDataSource struct {
	statService         Stat
	importerService     Importer
	dvcrSettings        *dvcr.Settings
	controllerNamespace string
	recorder            eventrecord.EventRecorderLogger
}

func NewHTTPDataSource(
	recorder eventrecord.EventRecorderLogger,
	statService Stat,
	importerService Importer,
	dvcrSettings *dvcr.Settings,
	controllerNamespace string,
) *HTTPDataSource {
	return &HTTPDataSource{
		statService:         statService,
		importerService:     importerService,
		dvcrSettings:        dvcrSettings,
		controllerNamespace: controllerNamespace,
		recorder:            recorder,
	}
}

func (ds HTTPDataSource) Sync(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "http")

	condition, _ := conditions.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
	cb := conditions.NewConditionBuilder(cvicondition.ReadyType).Generation(cvi.Generation)
	defer func() {
		// It is necessary to avoid setting unknown for the ready condition if it was already set to true.
		if !(cb.Condition().Status == metav1.ConditionUnknown && condition.Status == metav1.ConditionTrue) {
			conditions.SetCondition(cb, &cvi.Status.Conditions)
		}
	}()

	supgen := supplements.NewGenerator(annotations.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Cluster virtual image provisioning finished: clean up")

		cb.Status(metav1.ConditionTrue).
			Reason(cvicondition.Ready).
			Message("")

		cvi.Status.Phase = virtv2.ImageReady

		// Unprotect import time supplements to delete them later.
		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return reconcile.Result{}, err
		}

		return CleanUp(ctx, cvi, ds)
	case object.IsTerminating(pod):
		cvi.Status.Phase = virtv2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil:
		ds.recorder.Event(
			cvi,
			corev1.EventTypeNormal,
			virtv2.ReasonDataSourceSyncStarted,
			"The HTTP DataSource import has started",
		)

		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)

		envSettings := ds.getEnvSettings(cvi, supgen)
		err = ds.importerService.Start(ctx, envSettings, cvi, supgen, datasource.NewCABundleForCVMI(cvi.Spec.DataSource))
		switch {
		case err == nil:
			// OK.
		case common.ErrQuotaExceeded(err):
			ds.recorder.Event(cvi, corev1.EventTypeWarning, virtv2.ReasonDataSourceQuotaExceeded, "DataSource quota exceed")
			return setQuotaExceededPhaseCondition(cb, &cvi.Status.Phase, err, cvi.CreationTimestamp), nil
		default:
			setPhaseConditionToFailed(cb, &cvi.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, err
		}

		cvi.Status.Phase = virtv2.ImageProvisioning
		cb.Status(metav1.ConditionFalse).
			Reason(cvicondition.Provisioning).
			Message("DVCR Provisioner not found: create the new one.")

		log.Info("Create importer pod...", "progress", cvi.Status.Progress, "pod.phase", "nil")

		return reconcile.Result{Requeue: true}, nil
	case podutil.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			cvi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				ds.recorder.Event(cvi, corev1.EventTypeWarning, virtv2.ReasonDataSourceDiskProvisioningFailed, "Disk provisioning failed")
				cb.Status(metav1.ConditionFalse).
					Reason(cvicondition.ProvisioningFailed).
					Message(service.CapitalizeFirstLetter(err.Error() + "."))
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}

		ds.recorder.Event(
			cvi,
			corev1.EventTypeNormal,
			virtv2.ReasonDataSourceSyncCompleted,
			"The HTTP DataSource import has completed",
		)

		cb.Status(metav1.ConditionTrue).
			Reason(cvicondition.Ready).
			Message("")

		cvi.Status.Phase = virtv2.ImageReady
		cvi.Status.Size = ds.statService.GetSize(pod)
		cvi.Status.CDROM = ds.statService.GetCDROM(pod)
		cvi.Status.Format = ds.statService.GetFormat(pod)
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)
		cvi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(cvi.GetUID(), pod)

		log.Info("Ready", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	default:
		err = ds.statService.CheckPod(pod)
		if err != nil {
			cvi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrNotInitialized), errors.Is(err, service.ErrNotScheduled):
				cb.Status(metav1.ConditionFalse).
					Reason(cvicondition.ProvisioningNotStarted).
					Message(service.CapitalizeFirstLetter(err.Error() + "."))
				return reconcile.Result{}, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				ds.recorder.Event(cvi, corev1.EventTypeWarning, virtv2.ReasonDataSourceDiskProvisioningFailed, "Disk provisioning failed")
				cb.Status(metav1.ConditionFalse).
					Reason(cvicondition.ProvisioningFailed).
					Message(service.CapitalizeFirstLetter(err.Error() + "."))
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}

		err = ds.importerService.Protect(ctx, pod)
		if err != nil {
			return reconcile.Result{}, err
		}

		cb.Status(metav1.ConditionFalse).
			Reason(cvicondition.Provisioning).
			Message("Import is in the process of provisioning to DVCR.")

		cvi.Status.Phase = virtv2.ImageProvisioning
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)
		cvi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(cvi.GetUID(), pod)

		log.Info("Provisioning...", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return reconcile.Result{Requeue: true}, nil
}

func (ds HTTPDataSource) CleanUp(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error) {
	supgen := supplements.NewGenerator(annotations.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)

	requeue, err := ds.importerService.CleanUp(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{Requeue: requeue}, nil
}

func (ds HTTPDataSource) Validate(_ context.Context, _ *virtv2.ClusterVirtualImage) error {
	return nil
}

func (ds HTTPDataSource) getEnvSettings(cvi *virtv2.ClusterVirtualImage, supgen *supplements.Generator) *importer.Settings {
	var settings importer.Settings

	importer.ApplyHTTPSourceSettings(&settings, cvi.Spec.DataSource.HTTP, supgen)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForCVI(cvi),
	)

	return &settings
}
