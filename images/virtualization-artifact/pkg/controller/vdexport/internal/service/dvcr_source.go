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
	"io"
	"net/http"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vdexport/internal/service/factory"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	vdexportcondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vdexport-condition"
)

type DVCRExportSource struct {
	client       client.Client
	sup          *supplements.Generator
	newFactory   func(exportImage string) (factory.Factory, error)
	vdexport     *v1alpha2.VirtualDataExport
	dvcrSettings *dvcr.Settings
}

func NewDVCRExportSource(client client.Client, vdexport *v1alpha2.VirtualDataExport, cfg ExportSourceConfig) (DVCRExportSource, error) {
	sup := supplements.NewGenerator(annotations.VDExportShortName, vdexport.Name, vdexport.Namespace, vdexport.UID)
	return DVCRExportSource{
		client: client,
		sup:    sup,
		newFactory: func(exportImage string) (factory.Factory, error) {
			config := factory.Config{
				ControllerName:       "vdexport-controller",
				Image:                cfg.ExporterImage,
				ExportImage:          exportImage,
				Host:                 cfg.Dvcr.IngressSettings.Host,
				PullPolicy:           corev1.PullIfNotPresent,
				ResourceRequirements: &cfg.Requirements,
				WithCA:               cfg.Dvcr.CertsSecret != "" && cfg.Dvcr.CertsSecretNamespace != "",
			}
			if cfg.Dvcr.IngressSettings.Class != "" {
				config.ClassName = ptr.To(cfg.Dvcr.IngressSettings.Class)
			}
			if cfg.Dvcr.IngressSettings.TLSSecret != "" {
				config.TLSSecretName = ptr.To(cfg.Dvcr.IngressSettings.TLSSecret)
			}

			f, err := config.Complete(sup, vdexport)
			return f, err
		},
		vdexport:     vdexport,
		dvcrSettings: cfg.Dvcr,
	}, nil
}

func (d DVCRExportSource) Prepare(ctx context.Context) error {
	pod, svc, ing, err := d.getAll(ctx)
	if err != nil {
		return err
	}

	dvcrImage, internalErr, err := d.getImage()
	if err != nil {
		return err
	}
	if internalErr != nil {
		return nil
	}

	f, err := d.newFactory(dvcrImage)
	if err != nil {
		return err
	}

	if pod == nil {
		pod = f.Pod()
		if err = d.client.Create(ctx, pod); err != nil {
			return err
		}
	}
	err = supplements.EnsureForExporterPod(ctx, d.client, d.sup, pod, d.dvcrSettings)
	if err != nil {
		return err
	}

	if svc == nil {
		if err = d.client.Create(ctx, f.Service()); err != nil {
			return err
		}
	}

	if ing == nil {
		ing = f.Ingress()
		if err = d.client.Create(ctx, ing); err != nil {
			return err
		}
	}
	return supplements.EnsureForExporterIngress(ctx, d.client, d.sup, ing, d.dvcrSettings)
}

func (d DVCRExportSource) CleanUp(ctx context.Context) error {
	pod, svc, ing, err := d.getAll(ctx)
	if err != nil {
		return err
	}
	if pod != nil {
		if err = d.client.Delete(ctx, pod); err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
	}
	if svc != nil {
		if err = d.client.Delete(ctx, svc); err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
	}
	if ing != nil {
		if err = d.client.Delete(ctx, ing); err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (d DVCRExportSource) Status(ctx context.Context) (ExportStatus, error) {
	_, internalErr, err := d.getImage()
	if err != nil {
		return ExportStatus{}, err
	}
	if internalErr != nil {
		return ExportStatus{
			CompletedMessage: internalErr.Error(),
			CompletedReason:  vdexportcondition.ReasonFailed,
			CompletedStatus:  metav1.ConditionFalse,
		}, nil
	}

	pod, svc, ing, err := d.getAll(ctx)
	if err != nil {
		return ExportStatus{}, err
	}

	allCreated := pod != nil && svc != nil && ing != nil
	if !allCreated {
		return ExportStatus{
			CompletedMessage: "Resources for export are not created yet",
			CompletedReason:  vdexportcondition.ReasonPending,
			CompletedStatus:  metav1.ConditionFalse,
		}, nil
	}

	status := ExportStatus{
		URL: ing.GetAnnotations()[annotations.AnnExportURL],
	}

	switch pod.Status.Phase {
	case corev1.PodSucceeded:
		status.CompletedMessage = ""
		status.CompletedReason = vdexportcondition.ReasonCompleted
		status.CompletedStatus = metav1.ConditionTrue
		return status, nil
	case corev1.PodFailed:
		status.CompletedMessage = pod.Status.Reason
		status.CompletedReason = vdexportcondition.ReasonFailed
		status.CompletedStatus = metav1.ConditionFalse
		return status, nil
	case corev1.PodRunning: // break
	default:
		status.CompletedMessage = "Not all resources are ready for export"
		status.CompletedReason = vdexportcondition.ReasonPending
		status.CompletedStatus = metav1.ConditionFalse
		return status, nil
	}

	inProgress, err := d.isExportInProgress(svc)
	if err != nil {
		return ExportStatus{}, err
	}
	if inProgress {
		status.CompletedMessage = ""
		status.CompletedReason = vdexportcondition.ReasonInProgress
		status.CompletedStatus = metav1.ConditionFalse
		return status, nil
	}

	status.CompletedMessage = ""
	status.CompletedReason = vdexportcondition.ReasonWaitForUserDownload
	status.CompletedStatus = metav1.ConditionFalse

	return status, nil
}

func (d DVCRExportSource) Type() ExportType {
	return DVCR
}

func (d DVCRExportSource) getAll(ctx context.Context) (*corev1.Pod, *corev1.Service, *netv1.Ingress, error) {
	pod, err := object.FetchObject(ctx, d.sup.ExporterPod(), d.client, &corev1.Pod{})
	if err != nil {
		return nil, nil, nil, err
	}
	svc, err := object.FetchObject(ctx, d.sup.ExporterService(), d.client, &corev1.Service{})
	if err != nil {
		return nil, nil, nil, err
	}
	ing, err := object.FetchObject(ctx, d.sup.ExporterIngress(), d.client, &netv1.Ingress{})
	if err != nil {
		return nil, nil, nil, err
	}

	return pod, svc, ing, nil
}

func (d DVCRExportSource) isExportInProgress(svc *corev1.Service) (bool, error) {
	httpURL := fmt.Sprintf("http://%s.%s.svc:%d/inprogress-count", svc.Name, svc.Namespace, svc.Spec.Ports[0].Port)
	resp, err := http.Get(httpURL)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	if resp.Body != nil {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, err
		}
		res, err := strconv.Atoi(string(body))
		return err != nil && res > 0, nil
	}
	return false, nil
}

func (d DVCRExportSource) getImage() (string, error, error) {
	switch d.vdexport.Spec.TargetRef.Kind {
	case v1alpha2.VirtualDataExportTargetVirtualImage:
		vi := &v1alpha2.VirtualImage{}
		err := d.client.Get(context.Background(), client.ObjectKey{Namespace: d.vdexport.Namespace, Name: d.vdexport.Spec.TargetRef.Name}, vi)
		if err != nil {
			return "", nil, err
		}
		if vi.Spec.Storage != v1alpha2.StorageContainerRegistry {
			return "", fmt.Errorf("unsupported storage type %q", vi.Spec.Storage), nil
		}
		if vi.Status.Target.RegistryURL == "" {
			return "", fmt.Errorf("VirtualImage target registry url is empty"), nil
		}
		return vi.Status.Target.RegistryURL, nil, nil
	case v1alpha2.VirtualDataExportTargetClusterVirtualImage:
		cvi := &v1alpha2.ClusterVirtualImage{}
		err := d.client.Get(context.Background(), client.ObjectKey{Namespace: d.vdexport.Namespace, Name: d.vdexport.Spec.TargetRef.Name}, cvi)
		if err != nil {
			return "", nil, err
		}
		if cvi.Status.Target.RegistryURL == "" {
			return "", fmt.Errorf("ClusterVirtualImage target registry url is empty"), nil
		}
		return cvi.Status.Target.RegistryURL, nil, nil

	default:
		return "", nil, fmt.Errorf("unknown target kind %q", d.vdexport.Spec.TargetRef.Kind)
	}
}
