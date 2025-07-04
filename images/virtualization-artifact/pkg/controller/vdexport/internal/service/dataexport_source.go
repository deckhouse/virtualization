/*
Copyright 2025 Flant JSC

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
	"fmt"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vdexport/dataexport"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	vdexportcondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vdexport-condition"
)

func NewDataExportSource(client client.Client, vdexport *v1alpha2.VirtualDataExport) DataExportSource {
	return DataExportSource{
		client:   client,
		vdexport: vdexport,
	}
}

type DataExportSource struct {
	client   client.Client
	vdexport *v1alpha2.VirtualDataExport
}

func (d DataExportSource) Prepare(ctx context.Context) error {
	dataExport, err := d.getDataExport(ctx)

	switch {
	case err == nil:
		return nil
	case !k8serrors.IsNotFound(err):
		return err
	}

	// DataExport not found, need to create it
	vd, err := d.getVirtualDisk(ctx)
	if err != nil {
		return err
	}

	if vd.Status.Phase != v1alpha2.DiskReady {
		return nil
	}

	dataExport = dataexport.NewEmptyDataExport()
	dataExport.SetName(d.vdexport.Name)
	dataExport.SetNamespace(d.vdexport.Namespace)
	dataExport.SetTargetRef("PersistentVolumeClaim", vd.Status.Target.PersistentVolumeClaim)
	dataExport.SetTTL(d.vdexport.Spec.IdleTimeout.Duration.String())
	dataExport.SetPublish()

	return d.client.Create(ctx, dataExport)
}

func (d DataExportSource) CleanUp(ctx context.Context) error {
	dataExport, err := d.getDataExport(ctx)
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	return d.client.Delete(ctx, dataExport)
}

func (d DataExportSource) Status(ctx context.Context) (ExportStatus, error) {
	var status ExportStatus

	dataExport, err := d.getDataExport(ctx)
	switch {
	case err == nil:
	case k8serrors.IsNotFound(err):
		vd, err := d.getVirtualDisk(ctx)
		switch {
		case err == nil:
		case k8serrors.IsNotFound(err):
			status.CompletedMessage = fmt.Sprintf("VirtualDisk %q is not found", d.vdexport.Spec.TargetRef.Name)
			status.CompletedReason = vdexportcondition.ReasonPending
			status.CompletedStatus = metav1.ConditionFalse
			return status, nil
		default:
			return ExportStatus{}, err
		}

		if vd.Status.Phase != v1alpha2.DiskReady {
			status.CompletedMessage = "VirtualDisk is not ready"
			status.CompletedReason = vdexportcondition.ReasonPending
			status.CompletedStatus = metav1.ConditionFalse
			return status, nil
		}
		status.CompletedMessage = fmt.Sprintf("DataExport %q is not created yet, waiting", d.vdexport.Name)
		status.CompletedReason = vdexportcondition.ReasonPending
		status.CompletedStatus = metav1.ConditionFalse
		return status, nil
	default:
		return ExportStatus{}, err
	}

	status.URL = dataExport.GetStatusPublicURL()

	condReady, condExpired := getReadyAndExpiredConditions(dataExport.GetStatusConditions())

	if condExpired.Status == metav1.ConditionTrue {
		status.CompletedMessage = "VirtualDataExport is expired"
		status.CompletedReason = vdexportcondition.ReasonExpired
		status.CompletedStatus = metav1.ConditionFalse
		return status, nil
	}

	if condReady.Status == metav1.ConditionFalse {
		status.CompletedMessage = "VirtualDataExport is not ready, waiting a DataExport to be ready"
		status.CompletedReason = vdexportcondition.ReasonPending
		status.CompletedStatus = metav1.ConditionFalse
		return status, nil
	}

	accessTimestamp := dataExport.GetStatusAccessTimestamp()
	if accessTimestamp.IsZero() {
		status.CompletedMessage = "VirtualDataExport ready for export. Check .status.url for the export VirtualDisk"
		status.CompletedReason = vdexportcondition.ReasonWaitForUserDownload
		status.CompletedStatus = metav1.ConditionFalse
		return status, nil
	}

	// DataExport check progress every 30 seconds. That's why we check if the time is more than 45 seconds, then export done.
	done := time.Now().Sub(accessTimestamp.Time) > time.Second*45

	if !done {
		status.CompletedMessage = ""
		status.CompletedReason = vdexportcondition.ReasonInProgress
		status.CompletedStatus = metav1.ConditionFalse
		return status, nil
	}

	status.CompletedMessage = ""
	status.CompletedReason = vdexportcondition.ReasonCompleted
	status.CompletedStatus = metav1.ConditionTrue

	return status, nil
}

func (d DataExportSource) Type() ExportType {
	return DataExport
}

func (d DataExportSource) getDataExport(ctx context.Context) (*dataexport.DataExport, error) {
	dataExport := dataexport.NewEmptyDataExport()
	err := d.client.Get(ctx, client.ObjectKeyFromObject(d.vdexport), dataExport)
	return dataExport, err
}

func (d DataExportSource) getVirtualDisk(ctx context.Context) (*v1alpha2.VirtualDisk, error) {
	vd := &v1alpha2.VirtualDisk{}
	err := d.client.Get(ctx, client.ObjectKey{Namespace: d.vdexport.Namespace, Name: d.vdexport.Spec.TargetRef.Name}, vd)
	return vd, err
}

func getReadyAndExpiredConditions(conditions []metav1.Condition) (metav1.Condition, metav1.Condition) {
	var (
		condReady   metav1.Condition
		condExpired metav1.Condition
	)
	for _, cond := range conditions {
		switch cond.Type {
		case "Ready":
			condReady = cond
		case "Expired":
			condExpired = cond
		}
	}

	return condReady, condExpired
}
