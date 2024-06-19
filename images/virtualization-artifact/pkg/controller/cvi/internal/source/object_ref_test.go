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

package source

// var _ = Describe("ObjectRefDataSource Run", func() {
//	expectedStatus := getExpectedStatus()
//	stat := getStatMock(expectedStatus)
//
//	var cvi *virtv2.ClusterVirtualImage
//
//	BeforeEach(func() {
//		cvi = &virtv2.ClusterVirtualImage{}
//		cvi.Spec.DataSource.Type = virtv2.DataSourceTypeObjectRef
//		cvi.Spec.DataSource.ObjectRef = &virtv2.ClusterVirtualImageObjectRef{
//			Kind: virtv2.ClusterVirtualImageObjectRefKindClusterVirtualImage,
//		}
//	})
//
//	It("To provisioning phase (no importer pod)", func() {
//		impt := ImporterMock{
//			GetPodFunc: func(ctx context.Context, sup *supplements.Generator) (*corev1.Pod, error) {
//				return nil, nil
//			},
//			StartFunc: func(ctx context.Context, settings *importer.Settings, obj service.ObjectKind, sup *supplements.Generator, caBundle *datasource.CABundle) error {
//				return nil
//			},
//		}
//
//		ds := NewObjectRefDataSource(stat, &impt, &dvcr.Settings{}, nil, "")
//
//		requeue, err := ds.Sync(context.Background(), cvi)
//		Expect(err).To(BeNil())
//		Expect(requeue).To(BeTrue())
//		Expect(cvi.Status.Phase).To(Equal(virtv2.ImageProvisioning))
//		Expect(cvi.Status.Target.RegistryURL).NotTo(BeEmpty())
//	})
//
//	It("To provisioning phase (with importer pod)", func() {
//		impt := ImporterMock{
//			GetPodFunc: func(ctx context.Context, sup *supplements.Generator) (*corev1.Pod, error) {
//				return &corev1.Pod{
//					Status: corev1.PodStatus{
//						Phase: corev1.PodRunning,
//					},
//				}, nil
//			},
//		}
//
//		ds := NewObjectRefDataSource(stat, &impt, &dvcr.Settings{}, nil, "")
//
//		requeue, err := ds.Sync(context.Background(), cvi)
//		Expect(err).To(BeNil())
//		Expect(requeue).To(BeTrue())
//		Expect(cvi.Status.Phase).To(Equal(virtv2.ImageProvisioning))
//		Expect(cvi.Status.Target.RegistryURL).NotTo(BeEmpty())
//	})
//
//	It("To ready phase", func() {
//		impt := ImporterMock{
//			GetPodFunc: func(ctx context.Context, sup *supplements.Generator) (*corev1.Pod, error) {
//				return &corev1.Pod{
//					Status: corev1.PodStatus{
//						Phase: corev1.PodSucceeded,
//					},
//				}, nil
//			},
//		}
//
//		ds := NewObjectRefDataSource(stat, &impt, &dvcr.Settings{}, nil, "")
//
//		requeue, err := ds.Sync(context.Background(), cvi)
//		Expect(err).To(BeNil())
//		Expect(requeue).To(BeTrue())
//		Expect(cvi.Status.Phase).To(Equal(virtv2.ImageReady))
//		Expect(cvi.Status.Size).To(Equal(expectedStatus.Size))
//		Expect(cvi.Status.CDROM).To(Equal(expectedStatus.CDROM))
//		Expect(cvi.Status.Format).To(Equal(expectedStatus.Format))
//		Expect(cvi.Status.Progress).To(Equal("100%"))
//		Expect(cvi.Status.Target.RegistryURL).NotTo(BeEmpty())
//	})
//
//	It("Clean up", func() {
//		cvi.Status.Phase = virtv2.ImageReady
//		impt := ImporterMock{
//			CleanUpFunc: func(ctx context.Context, sup *supplements.Generator) (bool, error) {
//				return true, nil
//			},
//		}
//
//		ds := NewObjectRefDataSource(stat, &impt, &dvcr.Settings{}, nil, "")
//
//		requeue, err := ds.Sync(context.Background(), cvi)
//		Expect(err).To(BeNil())
//		Expect(requeue).To(BeTrue())
//	})
// })
