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

package service

import (
	"fmt"
	"log/slog"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service/internal"
	"github.com/deckhouse/virtualization-controller/pkg/util"
)

var _ = Describe("StatService method GetSize", func() {
	var pod *corev1.Pod
	BeforeEach(func() {
		pod = &corev1.Pod{
			TypeMeta:   metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{},
			Spec:       corev1.PodSpec{},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{},
						},
					},
				},
			},
		}
	})

	It("Adjust unpacked virtual size", func() {
		const sourceSize = 1024
		const sourceVirtualSize = 2048
		terminationMsg := fmt.Sprintf(`{ "source-image-size": %d, "source-image-virtual-size": %d }`, sourceSize, sourceVirtualSize)
		expectedAdjustedVirtualSize := internal.AdjustImageSize(resource.MustParse(strconv.FormatUint(sourceVirtualSize, 10)))

		pod.Status.ContainerStatuses[0].State.Terminated.Message = terminationMsg

		s := NewStatService(slog.Default())
		size := s.GetSize(pod)
		Expect(size.Stored).To(Equal(util.HumanizeIBytes(sourceSize)))
		Expect(size.StoredBytes).To(Equal(strconv.FormatUint(sourceSize, 10)))
		Expect(size.Unpacked).To(Equal(util.HumanizeIBytes(uint64(expectedAdjustedVirtualSize.Value()))))
		Expect(size.UnpackedBytes).To(Equal(strconv.FormatUint(uint64(expectedAdjustedVirtualSize.Value()), 10)))
	})

	It("Adjust empty unpacked virtual size", func() {
		pod.Status.ContainerStatuses[0].State.Terminated.Message = "{}"

		s := NewStatService(slog.Default())
		size := s.GetSize(pod)
		Expect(size.Stored).To(Equal("0B"))
		Expect(size.StoredBytes).To(Equal("0"))
		Expect(size.Unpacked).To(Equal("0B"))
		Expect(size.UnpackedBytes).To(Equal("0"))
	})
})
