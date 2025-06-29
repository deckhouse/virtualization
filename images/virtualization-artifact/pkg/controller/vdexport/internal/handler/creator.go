package handler

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vdexport/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ExportSourceCreator func(client client.Client, vdexport *v1alpha2.VirtualDataExport) (service.ExportSource, error)

func NewExportSourceCreator(exporterImage string,
	requirements corev1.ResourceRequirements,
	dvcr *dvcr.Settings,
	controllerNamespace string,
) ExportSourceCreator {
	cfg := service.ExportSourceConfig{
		ExporterImage:       exporterImage,
		Requirements:        requirements,
		Dvcr:                dvcr,
		ControllerNamespace: controllerNamespace,
	}

	return func(client client.Client, vdexport *v1alpha2.VirtualDataExport) (service.ExportSource, error) {
		return service.NewExportSource(client, vdexport, cfg)
	}
}
