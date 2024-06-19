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

// var _ = Describe("UploadDataSource Run", func() {
//	expectedStatus := getExpectedStatus()
//	stat := getStatMock(expectedStatus)
//
//	var uplr *UploaderMock
//
//	var cvi *virtv2.ClusterVirtualImage
//
//	BeforeEach(func() {
//		cvi = &virtv2.ClusterVirtualImage{}
//		cvi.Spec.DataSource.Type = virtv2.DataSourceTypeUpload
//		uplr = &UploaderMock{
//			GetPodFunc: func(ctx context.Context, sup *supplements.Generator) (*corev1.Pod, error) {
//				return &corev1.Pod{}, nil
//			},
//			GetServiceFunc: func(ctx context.Context, sup *supplements.Generator) (*corev1.Service, error) {
//				return &corev1.Service{}, nil
//			},
//			GetIngressFunc: func(ctx context.Context, sup *supplements.Generator) (*netv1.Ingress, error) {
//				return &netv1.Ingress{}, nil
//			},
//		}
//	})
//
//	It("To pending phase (no importer pod)", func() {
//		uplr = &UploaderMock{
//			GetPodFunc: func(ctx context.Context, sup *supplements.Generator) (*corev1.Pod, error) {
//				return nil, nil
//			},
//			GetServiceFunc: func(ctx context.Context, sup *supplements.Generator) (*corev1.Service, error) {
//				return nil, nil
//			},
//			GetIngressFunc: func(ctx context.Context, sup *supplements.Generator) (*netv1.Ingress, error) {
//				return nil, nil
//			},
//			StartFunc: func(ctx context.Context, settings *uploader.Settings, obj service.ObjectKind, sup *supplements.Generator, caBundle *datasource.CABundle) error {
//				return nil
//			},
//		}
//
//		ds := NewUploadDataSource(stat, uplr, &dvcr.Settings{}, "")
//
//		requeue, err := ds.Sync(context.Background(), cvi)
//		Expect(err).To(BeNil())
//		Expect(requeue).To(BeTrue())
//		Expect(cvi.Status.Phase).To(Equal(virtv2.ImagePending))
//		Expect(cvi.Status.Target.RegistryURL).NotTo(BeEmpty())
//	})
//
//	It("To wait wo user upload phase", func() {
//		uplr.GetPodFunc = func(ctx context.Context, sup *supplements.Generator) (*corev1.Pod, error) {
//			return &corev1.Pod{
//				Status: corev1.PodStatus{Phase: corev1.PodRunning},
//			}, nil
//		}
//
//		stat.IsUploadStartedFunc = func(ownerUID types.UID, pod *corev1.Pod) bool {
//			return false
//		}
//
//		ds := NewUploadDataSource(stat, uplr, &dvcr.Settings{}, "")
//
//		requeue, err := ds.Sync(context.Background(), cvi)
//		Expect(err).To(BeNil())
//		Expect(requeue).To(BeTrue())
//		Expect(cvi.Status.Phase).To(Equal(virtv2.ImageWaitForUserUpload))
//		Expect(cvi.Status.Target.RegistryURL).NotTo(BeEmpty())
//	})
//
//	It("To provisioning phase", func() {
//		uplr.GetPodFunc = func(ctx context.Context, sup *supplements.Generator) (*corev1.Pod, error) {
//			return &corev1.Pod{
//				Status: corev1.PodStatus{Phase: corev1.PodRunning},
//			}, nil
//		}
//
//		stat.IsUploadStartedFunc = func(ownerUID types.UID, pod *corev1.Pod) bool {
//			return true
//		}
//
//		ds := NewUploadDataSource(stat, uplr, &dvcr.Settings{}, "")
//
//		requeue, err := ds.Sync(context.Background(), cvi)
//		Expect(err).To(BeNil())
//		Expect(requeue).To(BeTrue())
//		Expect(cvi.Status.Phase).To(Equal(virtv2.ImageProvisioning))
//		Expect(cvi.Status.Progress).To(Equal(expectedStatus.Progress))
//		Expect(cvi.Status.DownloadSpeed).To(Equal(expectedStatus.DownloadSpeed))
//		Expect(cvi.Status.Target.RegistryURL).NotTo(BeEmpty())
//	})
//
//	It("To ready phase", func() {
//		uplr.GetPodFunc = func(ctx context.Context, sup *supplements.Generator) (*corev1.Pod, error) {
//			return &corev1.Pod{
//				Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
//			}, nil
//		}
//
//		ds := NewUploadDataSource(stat, uplr, &dvcr.Settings{}, "")
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
//		uplr := UploaderMock{
//			CleanUpFunc: func(ctx context.Context, sup *supplements.Generator) (bool, error) {
//				return true, nil
//			},
//		}
//
//		ds := NewUploadDataSource(stat, &uplr, &dvcr.Settings{}, "")
//
//		requeue, err := ds.Sync(context.Background(), cvi)
//		Expect(err).To(BeNil())
//		Expect(requeue).To(BeTrue())
//	})
// })
