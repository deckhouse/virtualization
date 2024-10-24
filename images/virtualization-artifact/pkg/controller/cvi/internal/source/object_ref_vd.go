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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

type ObjectRefVirtualDisk struct {
	importerService     Importer
	client              client.Client
	statService         Stat
	dvcrSettings        *dvcr.Settings
	controllerNamespace string
}

func NewObjectRefVirtualDisk(importerService Importer, client client.Client, controllerNamespace string, dvcrSettings *dvcr.Settings, statService Stat) *ObjectRefVirtualDisk {
	return &ObjectRefVirtualDisk{
		importerService:     importerService,
		statService:         statService,
		dvcrSettings:        dvcrSettings,
		client:              client,
		controllerNamespace: controllerNamespace,
	}
}

func (ds ObjectRefVirtualDisk) Sync(ctx context.Context, cvi *virtv2.ClusterVirtualImage, vdRef *virtv2.VirtualDisk, condition *metav1.Condition) (reconcile.Result, error) {
	log, ctx := logger.GetDataSourceContext(ctx, "objectref")

	supgen := supplements.NewGenerator(cc.CVIShortName, cvi.Name, vdRef.Namespace, cvi.UID)
	pod, err := ds.importerService.GetPod(ctx, supgen)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch {
	case isDiskProvisioningFinished(*condition):
		log.Info("Cluster virtual image provisioning finished: clean up")

		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
		condition.Message = ""

		cvi.Status.Phase = virtv2.ImageReady

		err = ds.importerService.Unprotect(ctx, pod)
		if err != nil {
			return reconcile.Result{}, err
		}

		return CleanUp(ctx, cvi, ds)
	case cc.IsTerminating(pod):
		cvi.Status.Phase = virtv2.ImagePending

		log.Info("Cleaning up...")
	case pod == nil:
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		envSettings := ds.getEnvSettings(cvi, supgen)

		ownerRef := metav1.NewControllerRef(cvi, cvi.GroupVersionKind())
		podSettings := ds.importerService.GetPodSettingsWithPVC(ownerRef, supgen, vdRef.Status.Target.PersistentVolumeClaim, vdRef.Namespace)
		err = ds.importerService.StartWithPodSetting(ctx, envSettings, supgen, datasource.NewCABundleForCVMI(cvi.Spec.DataSource), podSettings)
		switch {
		case err == nil:
			// OK.
		case cc.ErrQuotaExceeded(err):
			return setQuotaExceededPhaseCondition(condition, &cvi.Status.Phase, err, cvi.CreationTimestamp), nil
		default:
			setPhaseConditionToFailed(condition, &cvi.Status.Phase, fmt.Errorf("unexpected error: %w", err))
			return reconcile.Result{}, err
		}

		cvi.Status.Phase = virtv2.ImageProvisioning
		condition.Status = metav1.ConditionFalse
		condition.Reason = cvicondition.Provisioning
		condition.Message = "DVCR Provisioner not found: create the new one."

		log.Info("Create importer pod...", "progress", cvi.Status.Progress, "pod.phase", "nil")

		return reconcile.Result{Requeue: true}, nil
	case cc.IsPodComplete(pod):
		err = ds.statService.CheckPod(pod)
		if err != nil {
			cvi.Status.Phase = virtv2.ImageFailed

			switch {
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}

		condition.Status = metav1.ConditionTrue
		condition.Reason = vicondition.Ready
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
				condition.Reason = vicondition.ProvisioningNotStarted
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return reconcile.Result{}, nil
			case errors.Is(err, service.ErrProvisioningFailed):
				condition.Status = metav1.ConditionFalse
				condition.Reason = vicondition.ProvisioningFailed
				condition.Message = service.CapitalizeFirstLetter(err.Error() + ".")
				return reconcile.Result{}, nil
			default:
				return reconcile.Result{}, err
			}
		}

		err = ds.importerService.Protect(ctx, pod)
		if err != nil {
			return reconcile.Result{}, err
		}

		condition.Status = metav1.ConditionFalse
		condition.Reason = vicondition.Provisioning
		condition.Message = "Import is in the process of provisioning to DVCR."

		cvi.Status.Phase = virtv2.ImageProvisioning
		cvi.Status.Progress = ds.statService.GetProgress(cvi.GetUID(), pod, cvi.Status.Progress)
		cvi.Status.Target.RegistryURL = ds.statService.GetDVCRImageName(pod)

		log.Info("Provisioning...", "progress", cvi.Status.Progress, "pod.phase", pod.Status.Phase)
	}

	return reconcile.Result{Requeue: true}, nil
}

func (ds ObjectRefVirtualDisk) CleanUp(ctx context.Context, cvi *virtv2.ClusterVirtualImage) (reconcile.Result, error) {
	importerRequeue, err := ds.importerService.DeletePod(ctx, cvi, controllerName)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{Requeue: importerRequeue}, nil
}

func (ds ObjectRefVirtualDisk) getEnvSettings(cvi *virtv2.ClusterVirtualImage, sup *supplements.Generator) *importer.Settings {
	var settings importer.Settings
	importer.ApplyBlockDeviceSourceSettings(&settings)
	importer.ApplyDVCRDestinationSettings(
		&settings,
		ds.dvcrSettings,
		sup,
		ds.dvcrSettings.RegistryImageForCVI(cvi),
	)

	return &settings
}

func (ds ObjectRefVirtualDisk) Validate(ctx context.Context, cvi *virtv2.ClusterVirtualImage) error {
	if cvi.Spec.DataSource.ObjectRef == nil || cvi.Spec.DataSource.ObjectRef.Kind != virtv2.ClusterVirtualImageObjectRefKindVirtualDisk {
		return fmt.Errorf("not a %s data source", virtv2.ClusterVirtualImageObjectRefKindVirtualDisk)
	}

	vd, err := helper.FetchObject(ctx, types.NamespacedName{Name: cvi.Spec.DataSource.ObjectRef.Name, Namespace: cvi.Spec.DataSource.ObjectRef.Namespace}, ds.client, &virtv2.VirtualDisk{})
	if err != nil {
		return err
	}

	if vd == nil || vd.Status.Phase != virtv2.DiskReady {
		return NewVirtualDiskNotReadyError(cvi.Spec.DataSource.ObjectRef.Name)
	}

	if len(vd.Status.AttachedToVirtualMachines) != 0 {
		vmName := vd.Status.AttachedToVirtualMachines[0]

		vm, err := helper.FetchObject(ctx, types.NamespacedName{Name: vmName.Name, Namespace: vd.Namespace}, ds.client, &virtv2.VirtualMachine{})
		if err != nil {
			return err
		}

		if vm.Status.Phase != virtv2.MachineStopped {
			return NewVirtualDiskAttachedToRunningVMError(vd.Name, vmName.Name)
		}
	}

	return nil
}
