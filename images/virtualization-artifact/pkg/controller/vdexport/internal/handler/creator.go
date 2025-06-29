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
