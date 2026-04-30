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

package source

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

var _ = Describe("ObjectRef VirtualImage on PVC", func() {
	It("copies size from VirtualImage when provisioning pod is completed", func() {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodSucceeded,
			},
		}

		importer := &ImporterMock{
			GetPodFunc: func(_ context.Context, _ supplements.Generator) (*corev1.Pod, error) {
				return pod, nil
			},
		}
		stat := &StatMock{
			CheckPodFunc: func(_ *corev1.Pod) error {
				return nil
			},
			GetDVCRImageNameFunc: func(_ *corev1.Pod) string {
				return "registry.example.com/image:test"
			},
		}
		recorder := &eventrecord.EventRecorderLoggerMock{
			EventFunc: func(_ client.Object, _, _, _ string) {},
		}

		syncer := NewObjectRefVirtualImageOnPvc(recorder, importer, &dvcr.Settings{}, stat)

		vi := &v1alpha2.VirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vi",
				Namespace: "test-ns",
			},
			Status: v1alpha2.VirtualImageStatus{
				Size: v1alpha2.ImageStatusSize{
					Stored:        "12Gi",
					StoredBytes:   "12884901888",
					Unpacked:      "10Gi",
					UnpackedBytes: "10737418240",
				},
				CDROM:  true,
				Format: "raw",
				Target: v1alpha2.VirtualImageStatusTarget{
					PersistentVolumeClaim: "vi-pvc",
				},
			},
		}

		cvi := &v1alpha2.ClusterVirtualImage{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "cvi",
				Generation: 1,
				UID:        "11111111-1111-1111-1111-111111111111",
			},
		}

		cb := conditions.NewConditionBuilder(cvicondition.ReadyType).Generation(cvi.Generation)

		res, err := syncer.Sync(context.Background(), cvi, vi, cb)

		Expect(err).NotTo(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(time.Second))
		Expect(cvi.Status.Phase).To(Equal(v1alpha2.ImageReady))
		Expect(cvi.Status.Size).To(Equal(vi.Status.Size))
		Expect(cvi.Status.CDROM).To(Equal(vi.Status.CDROM))
		Expect(cvi.Status.Format).To(Equal(vi.Status.Format))
		Expect(cvi.Status.Target.RegistryURL).To(Equal("registry.example.com/image:test"))
	})
})
