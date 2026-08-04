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

package service

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("Cleanup", func() {
	Describe("CleanUpReason", func() {
		It("names the role and the namespaced object", func() {
			Expect(CleanUpReason(CleanUpRoleImporterPod, types.NamespacedName{Namespace: "default", Name: "vd-importer-disk"})).
				To(Equal(`waiting for the importer Pod "default/vd-importer-disk" to be deleted`))
		})

		It("omits the namespace for a cluster-scoped object", func() {
			Expect(CleanUpReason(CleanUpRolePersistentVolumeClaim, types.NamespacedName{Name: "pvc"})).
				To(Equal(`waiting for the PersistentVolumeClaim "pvc" to be deleted`))
		})

		It("falls back to the role alone when the name is unknown", func() {
			Expect(CleanUpReason(CleanUpRoleUploaderIngress, types.NamespacedName{})).
				To(Equal("waiting for the uploader Ingress to be deleted"))
		})

		It("returns an empty reason without a role", func() {
			Expect(CleanUpReason("", types.NamespacedName{Name: "pod"})).To(BeEmpty())
		})
	})

	Describe("CleanUpReasonForObject", func() {
		It("builds the reason from the object metadata", func() {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "vi-uploader-image"}}
			Expect(CleanUpReasonForObject(CleanUpRoleUploaderPod, pod)).
				To(Equal(`waiting for the uploader Pod "default/vi-uploader-image" to be deleted`))
		})

		It("returns an empty reason for a missing object", func() {
			Expect(CleanUpReasonForObject(CleanUpRoleUploaderPod, nil)).To(BeEmpty())
		})
	})

	Describe("MergeCleanUpReasons", func() {
		It("skips empty reasons and keeps unique reasons in order", func() {
			pvc := CleanUpReason(CleanUpRolePersistentVolumeClaim, types.NamespacedName{Namespace: "default", Name: "vd-disk"})
			pod := CleanUpReason(CleanUpRoleImporterPod, types.NamespacedName{Namespace: "default", Name: "vd-importer-disk"})

			Expect(MergeCleanUpReasons("", pvc, pvc, pod)).To(Equal(pvc + "; " + pod))
		})

		It("keeps reasons for distinct objects of the same kind", func() {
			svc := CleanUpReason(CleanUpRoleUploaderService, types.NamespacedName{Namespace: "default", Name: "vi-uploader-image"})
			ing := CleanUpReason(CleanUpRoleUploaderIngress, types.NamespacedName{Namespace: "default", Name: "vi-uploader-image"})

			Expect(MergeCleanUpReasons(svc, ing)).To(Equal(svc + "; " + ing))
		})

		It("returns an empty string when every reason is empty", func() {
			Expect(MergeCleanUpReasons("", "", "")).To(Equal(""))
		})
	})

	Describe("SetTerminatingCondition", func() {
		It("reports the deletion as in progress with Status=True", func() {
			var conds []metav1.Condition
			SetTerminatingCondition(&conds, vdcondition.TerminatingType, vdcondition.CleanupPending, 7,
				CleanUpReason(CleanUpRoleImporterPod, types.NamespacedName{Namespace: "default", Name: "vd-importer-disk"}))

			Expect(conds).To(HaveLen(1))
			Expect(conds[0].Type).To(Equal(vdcondition.TerminatingType.String()))
			Expect(conds[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(conds[0].Reason).To(Equal(vdcondition.CleanupPending.String()))
			Expect(conds[0].ObservedGeneration).To(Equal(int64(7)))
			Expect(conds[0].Message).To(Equal(`Waiting for the importer Pod "default/vd-importer-disk" to be deleted.`))
		})

		It("truncates an oversized message and still terminates it with a period", func() {
			var conds []metav1.Condition
			SetTerminatingCondition(&conds, vdcondition.TerminatingType, vdcondition.CleanupPending, 1,
				strings.Repeat("a", maxConditionMessageLength+100))

			Expect([]rune(conds[0].Message)).To(HaveLen(maxConditionMessageLength))
			Expect(conds[0].Message).To(HaveSuffix(" (truncated)."))
		})
	})

	Describe("truncateMessage", func() {
		It("keeps a message that fits within the limit unchanged", func() {
			Expect(truncateMessage("short message")).To(Equal("short message"))
		})

		It("keeps a message of exactly the limit unchanged", func() {
			s := strings.Repeat("a", maxConditionMessageLength)
			Expect(truncateMessage(s)).To(Equal(s))
		})

		It("marks the truncation and keeps the terminating period", func() {
			s := strings.Repeat("a", maxConditionMessageLength+100)
			got := truncateMessage(s)
			Expect([]rune(got)).To(HaveLen(maxConditionMessageLength))
			Expect(got).To(HaveSuffix(" (truncated)."))
			Expect(got).To(HaveSuffix("."))
		})

		It("never cuts inside a quoted name", func() {
			// The cut lands in the middle of the last name, which must be dropped whole
			// instead of leaving an unbalanced quote behind.
			names := make([]string, 0, 40)
			for i := 0; i < 40; i++ {
				names = append(names, fmt.Sprintf("%q", fmt.Sprintf("namespace-%02d/virtual-machine-%02d", i, i)))
			}

			got := truncateMessage("In use by: " + strings.Join(names, ", ") + ".")

			Expect(len([]rune(got))).To(BeNumerically("<=", maxConditionMessageLength))
			Expect(got).To(HaveSuffix(`" (truncated).`))
			Expect(strings.Count(got, `"`)%2).To(Equal(0), "quotes must stay balanced: %s", got)
			// No separator is left dangling in front of the marker.
			Expect(got).NotTo(MatchRegexp(`[,;] \(truncated\)\.$`))
		})

		It("drops the trailing clause instead of cutting a word in half", func() {
			clause := func(i int) string {
				return CleanUpReason(CleanUpRoleImporterPod,
					types.NamespacedName{Namespace: "default", Name: fmt.Sprintf("d8v-vd-importer-%048d", i)})
			}

			got := truncateMessage(CapitalizeFirstLetter(MergeCleanUpReasons(clause(1), clause(2), clause(3))) + ".")

			Expect(len([]rune(got))).To(BeNumerically("<=", maxConditionMessageLength))
			Expect(got).To(HaveSuffix("to be deleted (truncated)."))
		})

		It("keeps the hard cut when a single quoted name exceeds the whole limit", func() {
			got := truncateMessage(`"` + strings.Repeat("a", maxConditionMessageLength+100) + `"`)

			Expect([]rune(got)).To(HaveLen(maxConditionMessageLength))
			Expect(got).To(HaveSuffix(" (truncated)."))
		})
	})
})
