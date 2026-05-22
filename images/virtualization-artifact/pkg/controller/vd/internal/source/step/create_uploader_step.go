/*
Copyright 2026 Flant JSC

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

package step

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type CreateUploaderStepUploaderService interface {
	Start(ctx context.Context, settings *uploader.Settings, obj client.Object, sup supplements.Generator, caBundle *datasource.CABundle, opts ...service.Option) error
}

type CreateUploaderStep struct {
	pvc          *corev1.PersistentVolumeClaim
	pod          *corev1.Pod
	svc          *corev1.Service
	ing          *netv1.Ingress
	uploader     CreateUploaderStepUploaderService
	dvcrSettings *dvcr.Settings
	recorder     eventrecord.EventRecorderLogger
	cb           *conditions.ConditionBuilder
}

func NewCreateUploaderStep(
	pvc *corev1.PersistentVolumeClaim,
	pod *corev1.Pod,
	svc *corev1.Service,
	ing *netv1.Ingress,
	uploader CreateUploaderStepUploaderService,
	dvcrSettings *dvcr.Settings,
	recorder eventrecord.EventRecorderLogger,
	cb *conditions.ConditionBuilder,
) *CreateUploaderStep {
	return &CreateUploaderStep{
		pvc:          pvc,
		pod:          pod,
		svc:          svc,
		ing:          ing,
		uploader:     uploader,
		dvcrSettings: dvcrSettings,
		recorder:     recorder,
		cb:           cb,
	}
}

func (s CreateUploaderStep) Take(ctx context.Context, vd *v1alpha2.VirtualDisk) (*reconcile.Result, error) {
	// Uploader is needed only until the underlying PVC is created.
	// Once the PVC exists the data has been uploaded to DVCR and the uploader
	// supplements are not recreated even if they are missing.
	if s.pvc != nil {
		return nil, nil
	}

	if s.pod != nil && s.svc != nil && s.ing != nil {
		return nil, nil
	}

	s.recorder.Event(
		vd,
		corev1.EventTypeNormal,
		v1alpha2.ReasonDataSourceSyncStarted,
		"The Upload DataSource import to DVCR has started",
	)

	vd.Status.Progress = "0%"

	supgen := vdsupplements.NewGenerator(vd)
	settings := s.getEnvSettings(vd, supgen)

	err := s.uploader.Start(
		ctx, settings, vd, supgen,
		datasource.NewCABundleForVMD(vd.GetNamespace(), vd.Spec.DataSource),
		service.WithSystemNodeToleration(),
	)
	switch {
	case err == nil:
		// OK.
	case common.ErrQuotaExceeded(err):
		s.recorder.Event(vd, corev1.EventTypeWarning, v1alpha2.ReasonDataSourceQuotaExceeded, "DataSource quota exceed")
		vd.Status.Phase = v1alpha2.DiskFailed
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vdcondition.QuotaExceeded).
			Message(fmt.Sprintf("Quota exceeded during the uploader provisioning: %s", err))
		return &reconcile.Result{}, nil
	default:
		return nil, fmt.Errorf("start uploader: %w", err)
	}

	vd.Status.Phase = v1alpha2.DiskProvisioning
	s.cb.
		Status(metav1.ConditionFalse).
		Reason(vdcondition.Provisioning).
		Message("DVCR Provisioner not found: create the new one.")

	return &reconcile.Result{RequeueAfter: time.Second}, nil
}

func (s CreateUploaderStep) getEnvSettings(vd *v1alpha2.VirtualDisk, supgen supplements.Generator) *uploader.Settings {
	var settings uploader.Settings

	uploader.ApplyDVCRDestinationSettings(
		&settings,
		s.dvcrSettings,
		supgen,
		s.dvcrSettings.RegistryImageForVD(vd),
	)

	return &settings
}
