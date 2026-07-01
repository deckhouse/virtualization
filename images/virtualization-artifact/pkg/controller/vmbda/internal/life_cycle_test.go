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

package internal

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

var _ = Describe("LifeCycleHandler helpers", func() {
	DescribeTable("isVirtualMachineMigrating", func(status metav1.ConditionStatus, hasCondition, expected bool) {
		vm := &v1alpha2.VirtualMachine{}
		if hasCondition {
			vm.Status.Conditions = []metav1.Condition{
				{
					Type:   vmcondition.TypeMigrating.String(),
					Status: status,
				},
			}
		}
		Expect(isVirtualMachineMigrating(vm)).To(Equal(expected))
	},
		Entry("migrating condition is true", metav1.ConditionTrue, true, true),
		Entry("migrating condition is false", metav1.ConditionFalse, true, false),
		Entry("migrating condition is unknown", metav1.ConditionUnknown, true, false),
		Entry("migrating condition is absent", metav1.ConditionTrue, false, false),
	)

	DescribeTable("isReadWriteOnce", func(accessModes []corev1.PersistentVolumeAccessMode, expected bool) {
		pvc := &corev1.PersistentVolumeClaim{
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: accessModes,
			},
		}
		Expect(isReadWriteOnce(pvc)).To(Equal(expected))
	},
		Entry("ReadWriteOnce", []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, true),
		Entry("ReadWriteOncePod", []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod}, true),
		Entry("ReadOnlyMany", []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany}, true),
		Entry("no access modes", []corev1.PersistentVolumeAccessMode{}, true),
		Entry("ReadWriteMany", []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}, false),
		Entry("ReadWriteMany among others", []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadWriteMany}, false),
	)
})
