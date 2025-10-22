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

package vd

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/monitoring/metrics/promutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type dataMetric struct {
	Name                    string
	Namespace               string
	UID                     string
	Phase                   v1alpha2.DiskPhase
	Labels                  map[string]string
	Annotations             map[string]string
	CapacityBytes           int64
	StorageClass            string
	PersistentVolumeClaim   string
	InUse                   bool
	AttachedVirtualMachines []string
}

// DO NOT mutate VirtualDisk!
func newDataMetric(vd *v1alpha2.VirtualDisk) *dataMetric {
	if vd == nil {
		return nil
	}

	capacityBytes := parseCapacityBytes(vd.Status.Capacity)
	inUse, attachedVMs := getInUseStatus(vd)

	return &dataMetric{
		Name:      vd.Name,
		Namespace: vd.Namespace,
		UID:       string(vd.UID),
		Phase:     vd.Status.Phase,
		Labels: promutil.WrapPrometheusLabels(vd.GetLabels(), "label", func(key, value string) bool {
			return false
		}),
		Annotations: promutil.WrapPrometheusLabels(vd.GetAnnotations(), "annotation", func(key, _ string) bool {
			return strings.HasPrefix(key, "kubectl.kubernetes.io")
		}),
		CapacityBytes:           capacityBytes,
		StorageClass:            vd.Status.StorageClassName,
		PersistentVolumeClaim:   vd.Status.Target.PersistentVolumeClaim,
		InUse:                   inUse,
		AttachedVirtualMachines: attachedVMs,
	}
}

func parseCapacityBytes(capacity string) int64 {
	if capacity == "" {
		return 0
	}
	q, err := resource.ParseQuantity(capacity)
	if err != nil {
		return 0
	}
    return q.Value()
}

func getInUseStatus(vd *v1alpha2.VirtualDisk) (bool, []string) {
	inUseCond, found := conditions.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
	if !found {
		return false, nil
	}

	if inUseCond.Status != metav1.ConditionTrue {
		return false, nil
	}

	vmNames := make([]string, 0, len(vd.Status.AttachedToVirtualMachines))
	for _, vm := range vd.Status.AttachedToVirtualMachines {
		vmNames = append(vmNames, vm.Name)
	}

	return true, vmNames
}
