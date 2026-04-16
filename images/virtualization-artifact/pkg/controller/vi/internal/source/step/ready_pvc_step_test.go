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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGetUnpackedSize(t *testing.T) {
	t.Run("uses requested size when it is set", func(t *testing.T) {
		pvc := &corev1.PersistentVolumeClaim{
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("12Gi"),
				},
			},
		}

		actual := getUnpackedSize(pvc)
		if actual.Cmp(resource.MustParse("10Gi")) != 0 {
			t.Fatalf("expected unpacked size 10Gi, got %s", actual.String())
		}
	})

	t.Run("falls back to pvc capacity when request is not set", func(t *testing.T) {
		pvc := &corev1.PersistentVolumeClaim{
			Status: corev1.PersistentVolumeClaimStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("12Gi"),
				},
			},
		}

		actual := getUnpackedSize(pvc)
		if actual.Cmp(resource.MustParse("12Gi")) != 0 {
			t.Fatalf("expected unpacked size 12Gi, got %s", actual.String())
		}
	})
}
