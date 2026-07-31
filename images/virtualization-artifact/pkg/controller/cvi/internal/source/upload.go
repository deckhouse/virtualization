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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	servicestat "github.com/deckhouse/virtualization-controller/pkg/controller/service/stat"
	serviceuploader "github.com/deckhouse/virtualization-controller/pkg/controller/service/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

type UploadDataSource struct {
	statService         Stat
	uploaderService     Uploader
	dvcrSettings        *dvcr.Settings
	controllerNamespace string
	recorder            eventrecord.EventRecorderLogger
	client              client.Client
}

func NewUploadDataSource(
	recorder eventrecord.EventRecorderLogger,
	statService Stat,
	uploaderService Uploader,
	dvcrSettings *dvcr.Settings,
	controllerNamespace string,
	client client.Client,
) *UploadDataSource {
	return &UploadDataSource{
		statService:         statService,
		uploaderService:     uploaderService,
		dvcrSettings:        dvcrSettings,
		controllerNamespace: controllerNamespace,
		recorder:            recorder,
		client:              client,
	}
}

func (ds UploadDataSource) Sync(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "upload")

	condition, _ := conditions.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
	cb := conditions.NewConditionBuilder(cvicondition.ReadyType).Generation(cvi.Generation)
	defer func() {
		// It is necessary to avoid setting unknown for the ready condition if it was already set to true.
		if cb.Condition().Status != metav1.ConditionUnknown || condition.Status != metav1.ConditionTrue {
			conditions.SetCondition(cb, &cvi.Status.Conditions)
		}
	}()

	supgen := supplements.NewGenerator(annotations.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)
	pod, err := ds.uploaderService.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}
	svc, err := ds.uploaderService.GetService(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}
	// Repair the external exposure before it is read and probed below. Initial
	// creation is Apply's job, so this only matters while the uploader is running.
	if pod != nil && !isDiskProvisioningFinished(condition) {
		if err = ds.uploaderService.EnsureExposure(ctx, cvi, supgen); err != nil {
			return reconcile.Result{}, err
		}
	}

	exposure, err := ds.uploaderService.GetExposure(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	isUploaderReady, err := ds.statService.IsUploaderReady(pod, svc, exposure)
	if err != nil {
		// A probe error means the public upload endpoint is not reachable yet
		// (e.g. TLS not settled after a secret restore). Treat as not-ready and
		// keep retrying instead of failing the reconcile with an empty status.
		log.Error("Uploader readiness probe failed; treating uploader as not ready", "err", err)
		isUploaderReady = false
	}

	switch {
	case isDiskProvisioningFinished(condition):
		log.Info("Cluster virtual image provisioning finished: clean up")

		cb.
			Status(metav1.ConditionTrue).
			Reason(cvicondition.Ready).
			Message("")

		cvi.Status.Phase = v1alpha2.ImageReady

		_, err = CleanUp(ctx, cvi, ds)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	case condition.Reason == cvicondition.WaitForUserUploadTimeout.String():
		log.Debug("Upload wait timed out: clean up")

		cvi.Status.Phase = v1alpha2.ImageFailed
		cvi.Status.ImageUploadURLs = nil
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.WaitForUserUploadTimeout).
			Message(serviceuploader.WaitForUserUploadTimeoutMessage("ClusterVirtualImage"))

		_, err = CleanUp(ctx, cvi, ds)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	case object.AnyTerminating(pod, svc):
		log.Info("Cleaning up...")
	case pod == nil || svc == nil || !exposure.Ensured():
		envSettings := ds.getEnvSettings(cvi, supgen)
		err = ds.uploaderService.Apply(ctx, cvi, supgen, envSettings, datasource.NewCABundleForCVMI(cvi.Spec.DataSource), serviceuploader.WithSystemNodeToleration())
		switch {
		case err == nil:
			// OK.
		case common.ErrQuotaExceeded(err):
			ds.recorder.Event(cvi, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceQuotaExceeded, "DataSource quota exceed")
			return setQuotaExceededPhaseCondition(cb, &cvi.Status.Phase, err, cvi.CreationTimestamp), nil
		default:
			setPhaseConditionToFailed(cb, &cvi.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, err
		}

		cvi.Status.Phase = v1alpha2.ImageProvisioning
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.Provisioning).
			Message("Preparing to import the image.")

		log.Info("Create uploader pod...", "progress", cvi.Status.Progress, "pod.phase", nil)

		return reconcile.Result{RequeueAfter: time.Second}, nil
	case podutil.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			recordProvisioningFailedEvent(ds.recorder, cvi, err)
			return reconcile.Result{}, setPhaseConditionFromPodError(cb, cvi, err)
		}

		ds.recorder.Event(
			cvi,
			corev1.EventTypeNormal,
			v1alpha2.ReasonDataSourceSyncCompleted,
			"The Upload DataSource import has completed",
		)

		cb.
			Status(metav1.ConditionTrue).
			Reason(cvicondition.Ready).
			Message("")

		cvi.Status.Phase = v1alpha2.ImageReady
		cvi.Status.Size = ds.statService.GetSize(pod)
		cvi.Status.CDROM = ds.statService.GetCDROM(pod)
		cvi.Status.Format = ds.statService.GetFormat(pod)
		cvi.Status.Progress = servicestat.ProgressDone
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)
		cvi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(cvi.GetUID(), pod)

		log.Info("Ready", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	case ds.statService.IsUploadStarted(cvi.GetUID(), pod) || hasUploadProgress(cvi.Status.Progress):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			if isTransientPodError(err) {
				setUploadProvisioningPhaseCondition(cb, cvi)
				return reconcile.Result{RequeueAfter: time.Second}, nil
			}
			recordProvisioningFailedEvent(ds.recorder, cvi, err)
			return reconcile.Result{}, setPhaseConditionFromPodError(cb, cvi, err)
		}

		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.Provisioning).
			Message("The image is being imported.")

		cvi.Status.Phase = v1alpha2.ImageProvisioning
		cvi.Status.Progress = servicestat.CapProgressBelow(ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress), servicestat.ProgressMax)
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)
		cvi.Status.DownloadSpeed = ds.statService.GetDownloadSpeed(cvi.GetUID(), pod)

		log.Info("Provisioning...", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	case condition.Reason == cvicondition.WaitForUserUpload.String() && serviceuploader.IsWaitForUserUploadTimeoutExpired(condition.LastTransitionTime):
		log.Info("Upload has not started in time: the import process has failed", "pod.name", pod.Name)
		ds.recorder.Event(cvi, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceSyncFailed, serviceuploader.WaitForUserUploadTimeoutMessage("ClusterVirtualImage"))

		cvi.Status.Phase = v1alpha2.ImageFailed
		cvi.Status.ImageUploadURLs = nil
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.WaitForUserUploadTimeout).
			Message(serviceuploader.WaitForUserUploadTimeoutMessage("ClusterVirtualImage"))

		_, err = CleanUp(ctx, cvi, ds)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	case isUploaderReady:
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.WaitForUserUpload).
			Message("Waiting for the image to be uploaded.")

		cvi.Status.Phase = v1alpha2.ImageWaitForUserUpload
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)
		cvi.Status.ImageUploadURLs = &v1alpha2.ImageUploadURLs{
			External:  exposure.UploadURL,
			InCluster: ds.uploaderService.GetInClusterURL(svc),
		}

		log.Info("Waiting for the image to be uploaded", "pod.phase", pod.Status.Phase)

		// Keep polling: the start of the upload is only visible through the pod
		// metrics scraped on reconcile.
		return reconcile.Result{RequeueAfter: serviceuploader.WaitForUserUploadRequeueAfter}, nil
	default:
		cb.
			Status(metav1.ConditionFalse).
			Reason(cvicondition.Provisioning).
			Message(fmt.Sprintf("Waiting for the uploader %q to be ready to process the user's upload.", pod.Name))

		cvi.Status.Phase = v1alpha2.ImageProvisioning

		log.Info("Waiting for the uploader to be ready to process the user's upload", "pod.phase", pod.Status.Phase)
	}

	// The upload is in progress (or the uploader is not serving yet): keep polling
	// every second so the reported progress and download speed stay live.
	return reconcile.Result{RequeueAfter: time.Second}, nil
}

func (ds UploadDataSource) CleanUp(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (bool, error) {
	supgen := supplements.NewGenerator(annotations.CVIShortName, cvi.Name, ds.controllerNamespace, cvi.UID)

	return ds.uploaderService.Cleanup(ctx, supgen)
}

func (ds UploadDataSource) Validate(_ context.Context, _ *v1alpha2.ClusterVirtualImage) error {
	return nil
}

func setUploadProvisioningPhaseCondition(cb *conditions.ConditionBuilder, cvi *v1alpha2.ClusterVirtualImage) {
	cvi.Status.Phase = v1alpha2.ImageProvisioning
	cb.
		Status(metav1.ConditionFalse).
		Reason(cvicondition.Provisioning).
		Message("The image is being imported.")
}

func isTransientPodError(err error) bool {
	return errors.Is(err, servicestat.ErrNotInitialized) || errors.Is(err, servicestat.ErrNotScheduled)
}

func hasUploadProgress(progress string) bool {
	switch progress {
	case "", "0", "0%", "0.0%", "0.00%":
		return false
	default:
		return true
	}
}

func (ds UploadDataSource) getEnvSettings(cvi *v1alpha2.ClusterVirtualImage, supgen supplements.Generator) serviceuploader.Settings {
	var settings serviceuploader.Settings

	serviceuploader.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		supgen,
		ds.dvcrSettings.RegistryImageForCVI(cvi),
	)

	return settings
}
