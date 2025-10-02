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

package vmbda

import (
	"strings"

	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/promutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type dataMetric struct {
	Name        string
	Namespace   string
	UID         string
	Phase       v1alpha2.BlockDeviceAttachmentPhase
	Labels      map[string]string
	Annotations map[string]string
}

// DO NOT mutate VirtualMachineBlockDeviceAttachment!
func newDataMetric(vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) *dataMetric {
	if vmbda == nil {
		return nil
	}

	return &dataMetric{
		Name:      vmbda.Name,
		Namespace: vmbda.Namespace,
		UID:       string(vmbda.UID),
		Phase:     vmbda.Status.Phase,
		Labels: promutil.WrapPrometheusLabels(vmbda.GetLabels(), "label", func(key, value string) bool {
			return false
		}),
		Annotations: promutil.WrapPrometheusLabels(vmbda.GetAnnotations(), "annotation", func(key, _ string) bool {
			return strings.HasPrefix(key, "kubectl.kubernetes.io")
		}),
	}
}
