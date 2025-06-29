package service

import (
	"context"
	"fmt"
	"io"
	"net/http"

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
	sup          *supplements.Generator
	factory      factory.Factory
	client       client.Client
	dvcrSettings *dvcr.Settings
}

func NewDVCRExportSource(client client.Client, vdexport *v1alpha2.VirtualDataExport, cfg ExportSourceConfig) (DVCRExportSource, error) {
	sup := supplements.NewGenerator(annotations.VDExportShortName, vdexport.Name, vdexport.Namespace, vdexport.UID)

	config := factory.Config{
		ControllerName:       "vdexport-controller",
		Image:                cfg.ExporterImage,
		DVCRAuthSecret:       cfg.Dvcr.AuthSecret,
		Host:                 cfg.Dvcr.IngressSettings.Host,
		PullPolicy:           corev1.PullIfNotPresent,
		ResourceRequirements: &cfg.Requirements,
	}
	if cfg.Dvcr.IngressSettings.Class != "" {
		config.ClassName = ptr.To(cfg.Dvcr.IngressSettings.Class)
	}
	if cfg.Dvcr.IngressSettings.TLSSecret != "" {
		config.TLSSecretName = ptr.To(cfg.Dvcr.IngressSettings.TLSSecret)
	}

	f, err := config.Complete(sup, vdexport)
	if err != nil {
		return DVCRExportSource{}, err
	}
	return DVCRExportSource{
		sup:          sup,
		factory:      f,
		client:       client,
		dvcrSettings: cfg.Dvcr,
	}, nil
}

func (d DVCRExportSource) Prepare(ctx context.Context) error {
	pod, svc, ing, err := d.getAll(ctx)
	if err != nil {
		return err
	}

	if pod == nil {
		pod = d.factory.Pod()
		if err = d.client.Create(ctx, pod); err != nil {
			return err
		}
	}
	err = supplements.EnsureForExporterPod(ctx, d.client, d.sup, pod, d.dvcrSettings)
	if err != nil {
		return err
	}

	if svc == nil {
		if err = d.client.Create(ctx, d.factory.Service()); err != nil {
			return err
		}
	}

	if ing == nil {
		ing = d.factory.Ingress()
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

	started, err := d.isExportStarted(svc)
	if err != nil {
		return ExportStatus{}, err
	}
	if started {
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

func (d DVCRExportSource) isExportStarted(svc *corev1.Service) (bool, error) {
	httpURL := fmt.Sprintf("http://%s.%s.svc:%d/started", svc.Name, svc.Namespace, svc.Spec.Ports[0].Port)
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
		return string(body) == "yes", nil
	}
	return false, nil
}
