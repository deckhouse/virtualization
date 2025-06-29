package service

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	vdexportcondition "github.com/deckhouse/virtualization/api/core/v1alpha2/vdexport-condition"
)

type ExportType int

const (
	DataExport ExportType = iota
	DVCR
)

func (t ExportType) String() string {
	switch t {
	case DataExport:
		return "DataExport"
	case DVCR:
		return "DVCR"
	default:
		return "Unknown"
	}
}

type ExportSource interface {
	Prepare(ctx context.Context) error
	CleanUp(ctx context.Context) error
	Status(ctx context.Context) (ExportStatus, error)
	Type() ExportType
}

type ExportStatus struct {
	URL              string
	CompletedMessage string
	CompletedReason  vdexportcondition.Reason
	CompletedStatus  metav1.ConditionStatus
}

func NewExportSource(client client.Client, vdexport *v1alpha2.VirtualDataExport, cfg ExportSourceConfig) (ExportSource, error) {
	if vdexport == nil {
		return nil, fmt.Errorf("vdexport must not be nil")
	}
	kind := vdexport.Spec.TargetRef.Kind
	switch kind {
	case v1alpha2.VirtualDataExportTargetVirtualDisk:
		return NewDataExportSource(client, vdexport), nil
	case v1alpha2.VirtualDataExportTargetVirtualImage, v1alpha2.VirtualDataExportTargetClusterVirtualImage:
		return NewDVCRExportSource(client, vdexport, cfg)
	default:
		return nil, fmt.Errorf("unsupported targetRef kind %q", kind)
	}
}

type ExportSourceConfig struct {
	ExporterImage       string
	Requirements        corev1.ResourceRequirements
	Dvcr                *dvcr.Settings
	ControllerNamespace string
}
