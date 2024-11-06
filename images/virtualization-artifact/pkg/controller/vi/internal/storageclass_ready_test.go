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

package internal

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("StorageClassHandler Run", func() {
	DescribeTable("Checking ActualStorageClass",
		func(storageClassFromModuleConfig string, vi *virtv2.VirtualImage, pvc *corev1.PersistentVolumeClaim, expectedStatus *string) {
			h := NewStorageClassReadyHandler(nil, storageClassFromModuleConfig)

			Expect(h.ActualStorageClass(vi, pvc)).To(Equal(expectedStatus))
		},
		Entry(
			"Should be nil if no VI",
			"",
			nil,
			nil,
			nil,
		),
		Entry(
			"Should be nil because no data",
			"",
			newVI(nil, ""),
			nil,
			nil,
		),
		Entry(
			"Should be from status if have in status, but not in spec",
			"",
			newVI(nil, "fromStatus"),
			nil,
			ptr.To("fromStatus"),
		),
		Entry(
			"Should be from status if have in status and in mc",
			"fromMC",
			newVI(nil, "fromStatus"),
			nil,
			ptr.To("fromStatus"),
		),
		Entry(
			"Should be from mc if only in mc",
			"fromMC",
			newVI(nil, ""),
			nil,
			ptr.To("fromMC"),
		),
		Entry(
			"Should be from spec if pvc does not exists",
			"fromMC",
			newVI(ptr.To("fromSPEC"), "fromStatus"),
			nil,
			ptr.To("fromSPEC"),
		),
		Entry(
			"Should be from spec if pvc exists, but his storage class is nil",
			"fromMC",
			newVI(ptr.To("fromSPEC"), "fromStatus"),
			newPVC(nil),
			ptr.To("fromSPEC"),
		),
		Entry(
			"Should be from spec if pvc exists, but his storage class is empty",
			"fromMC",
			newVI(ptr.To("fromSPEC"), "fromStatus"),
			newPVC(ptr.To("")),
			ptr.To("fromSPEC"),
		),
		Entry(
			"Should be from pvc if pvc exists",
			"fromMC",
			newVI(ptr.To("fromSPEC"), "fromStatus"),
			newPVC(ptr.To("fromPVC")),
			ptr.To("fromPVC"),
		),
	)
})

func newVI(scInSpec *string, scInStatus string) *virtv2.VirtualImage {
	return &virtv2.VirtualImage{
		Spec: virtv2.VirtualImageSpec{
			PersistentVolumeClaim: virtv2.VirtualImagePersistentVolumeClaim{
				StorageClass: scInSpec,
			},
		},
		Status: virtv2.VirtualImageStatus{
			StorageClassName: scInStatus,
		},
	}
}

func newPVC(sc *string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: sc,
		},
	}
}
