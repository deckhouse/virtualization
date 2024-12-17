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

package provisioner

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

func TestProvisioner(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Provisioner")
}

var _ = Describe("KeepNodePlacementTolerations", func() {
	var pod *corev1.Pod

	BeforeEach(func() {
		pod = &corev1.Pod{}
	})

	Context("there is no node placement", func() {
		It("doesnt set annotation", func() {
			err := KeepNodePlacementTolerations(nil, pod)
			Expect(err).To(BeNil())
			Expect(pod.Annotations).To(BeEmpty())
		})
	})

	Context("there are no tolerations", func() {
		It("doesnt set annotation", func() {
			var nodePlacement NodePlacement
			err := KeepNodePlacementTolerations(&nodePlacement, pod)
			Expect(err).To(BeNil())
			Expect(pod.Annotations).To(BeEmpty())
		})
	})

	Context("there are tolerations", func() {
		It("set annotation", func() {
			nodePlacement := NodePlacement{Tolerations: []corev1.Toleration{
				{
					Key:      "Foo",
					Operator: "Exists",
					Effect:   "NoSchedule",
				},
			}}

			err := KeepNodePlacementTolerations(&nodePlacement, pod)
			Expect(err).To(BeNil())
			Expect(pod.Annotations).ToNot(BeEmpty())
			Expect(pod.Annotations[annotations.AnnTolerationsHash]).ToNot(BeEmpty())
		})
	})
})

var _ = Describe("IsNodePlacementChanged", func() {
	var pod *corev1.Pod

	Context("there is no toleration in obj", func() {
		BeforeEach(func() {
			pod = &corev1.Pod{}
		})

		It("is not changed with empty node placement", func() {
			isChanged, err := IsNodePlacementChanged(nil, pod)
			Expect(err).To(BeNil())
			Expect(isChanged).To(BeFalse())
		})

		It("is not changed with empty tolerations", func() {
			isChanged, err := IsNodePlacementChanged(&NodePlacement{}, pod)
			Expect(err).To(BeNil())
			Expect(isChanged).To(BeFalse())
		})
	})

	Context("there is toleration in obj", func() {
		var nodePlacement *NodePlacement

		BeforeEach(func() {
			pod = &corev1.Pod{}

			nodePlacement = &NodePlacement{Tolerations: []corev1.Toleration{{Key: "Foo"}}}
			err := KeepNodePlacementTolerations(nodePlacement, pod)
			Expect(err).To(BeNil())
		})

		It("is not changed: with tolerations", func() {
			isChanged, err := IsNodePlacementChanged(nodePlacement, pod)
			Expect(err).To(BeNil())
			Expect(isChanged).To(BeFalse())
		})

		It("is changed: no node placement", func() {
			isChanged, err := IsNodePlacementChanged(nil, pod)
			Expect(err).To(BeNil())
			Expect(isChanged).To(BeTrue())
		})

		It("is changed: with empty tolerations", func() {
			isChanged, err := IsNodePlacementChanged(&NodePlacement{}, pod)
			Expect(err).To(BeNil())
			Expect(isChanged).To(BeTrue())
		})

		It("is changed: with different tolerations", func() {
			changedTolerations := &NodePlacement{Tolerations: []corev1.Toleration{{Key: "Bar"}}}
			isChanged, err := IsNodePlacementChanged(changedTolerations, pod)
			Expect(err).To(BeNil())
			Expect(isChanged).To(BeTrue())
		})
	})
})
