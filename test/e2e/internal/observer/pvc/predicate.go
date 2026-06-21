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

package pvc

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

func BeBound() Predicate {
	return func(pvc *corev1.PersistentVolumeClaim) (bool, error) {
		return pvc.Status.Phase == corev1.ClaimBound, nil
	}
}

func BeBoundAndPopulated() Predicate {
	return func(pvc *corev1.PersistentVolumeClaim) (bool, error) {
		return pvc.Status.Phase == corev1.ClaimBound &&
			pvc.Annotations[annotations.AnnPVCPopulationDone] == "true", nil
	}
}

func BeLost() Predicate {
	return func(pvc *corev1.PersistentVolumeClaim) (bool, error) {
		if pvc.Status.Phase == corev1.ClaimLost {
			return true, fmt.Errorf("PersistentVolumeClaim entered Lost phase")
		}
		return false, nil
	}
}
