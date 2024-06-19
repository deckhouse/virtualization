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

// func TestHTTPDataSource_Run(t *testing.T) {
//	expectedStatus := getExpectedStatus()
//	stat := getStatMock(expectedStatus)
//
//	t.Run("to provisioning phase (no importer pod)", func(t *testing.T) {
//		var cvi virtv2.ClusterVirtualImage
//		cvi.Spec.DataSource.Type = virtv2.DataSourceTypeHTTP
//		cvi.Spec.DataSource.HTTP = &virtv2.DataSourceHTTP{
//			URL: "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img",
//		}
//
//		impt := ImporterMock{
//			GetPodFunc: func(ctx context.Context, sup *supplements.Generator) (*corev1.Pod, error) {
//				return nil, nil
//			},
//			StartFunc: func(ctx context.Context, settings *importer.Settings, obj service.ObjectKind, sup *supplements.Generator, caBundle *datasource.CABundle) error {
//				return nil
//			},
//		}
//
//		ds := NewHTTPDataSource(stat, &impt, &dvcr.Settings{}, "")
//		requeue, err := ds.Sync(context.Background(), &cvi)
//		require.NoError(t, err)
//		require.True(t, requeue)
//
//		require.Equal(t, virtv2.ImageProvisioning, cvi.Status.Phase)
//		require.Equal(t, expectedStatus.Progress, cvi.Status.Progress)
//		require.Equal(t, expectedStatus.DownloadSpeed, cvi.Status.DownloadSpeed)
//		require.NotEmpty(t, cvi.Status.Target)
//	})
//
//	t.Run("to provisioning phase (with importer pod)", func(t *testing.T) {
//		var cvi virtv2.ClusterVirtualImage
//
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
//		ds := NewHTTPDataSource(stat, &impt, &dvcr.Settings{}, "")
//		requeue, err := ds.Sync(context.Background(), &cvi)
//		require.NoError(t, err)
//		require.True(t, requeue)
//
//		require.Equal(t, virtv2.ImageProvisioning, cvi.Status.Phase)
//		require.Equal(t, expectedStatus.Progress, cvi.Status.Progress)
//		require.Equal(t, expectedStatus.DownloadSpeed, cvi.Status.DownloadSpeed)
//		require.NotEmpty(t, cvi.Status.Target)
//	})
//
//	t.Run("to ready", func(t *testing.T) {
//		var cvi virtv2.ClusterVirtualImage
//
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
//		ds := NewHTTPDataSource(stat, &impt, &dvcr.Settings{}, "")
//		requeue, err := ds.Sync(context.Background(), &cvi)
//		require.NoError(t, err)
//		require.True(t, requeue)
//
//		require.Equal(t, virtv2.ImageReady, cvi.Status.Phase)
//		require.Equal(t, expectedStatus.Format, cvi.Status.Format)
//		require.Equal(t, expectedStatus.CDROM, cvi.Status.CDROM)
//		require.Equal(t, expectedStatus.Size, cvi.Status.Size)
//		require.Equal(t, expectedStatus.Progress, cvi.Status.Progress)
//		require.Equal(t, expectedStatus.DownloadSpeed, cvi.Status.DownloadSpeed)
//		require.NotEmpty(t, cvi.Status.Target)
//	})
//
//	t.Run("clean up", func(t *testing.T) {
//		var cvi virtv2.ClusterVirtualImage
//		cvi.Status = expectedStatus
//		cvi.Status.Phase = virtv2.ImageReady
//
//		impt := ImporterMock{
//			CleanUpFunc: func(ctx context.Context, sup *supplements.Generator) (bool, error) {
//				return false, nil
//			},
//		}
//
//		ds := NewHTTPDataSource(stat, &impt, &dvcr.Settings{}, "")
//		requeue, err := ds.Sync(context.Background(), &cvi)
//		require.NoError(t, err)
//		require.False(t, requeue)
//
//		require.Equal(t, virtv2.ImageReady, cvi.Status.Phase)
//		require.Equal(t, expectedStatus.Format, cvi.Status.Format)
//		require.Equal(t, expectedStatus.CDROM, cvi.Status.CDROM)
//		require.Equal(t, expectedStatus.Size, cvi.Status.Size)
//		require.Equal(t, expectedStatus.Progress, cvi.Status.Progress)
//		require.Equal(t, expectedStatus.DownloadSpeed, cvi.Status.DownloadSpeed)
//		require.NotEmpty(t, cvi.Status.Target)
//	})
// }

// func TestHTTPDataSource_CleanUp(t *testing.T) {
//	t.Run("clean up", func(t *testing.T) {
//		var cvi virtv2.ClusterVirtualImage
//
//		impt := ImporterMock{
//			CleanUpFunc: func(ctx context.Context, sup *supplements.Generator) (bool, error) {
//				return false, nil
//			},
//		}
//
//		ds := NewHTTPDataSource(nil, &impt, &dvcr.Settings{}, "")
//		requeue, err := ds.CleanUp(context.Background(), &cvi)
//		require.NoError(t, err)
//		require.False(t, requeue)
//	})
// }

// func getExpectedStatus() virtv2.ClusterVirtualImageStatus {
//	return virtv2.ClusterVirtualImageStatus{
//		ImageStatus: virtv2.ImageStatus{
//			Phase: virtv2.ImagePending,
//			DownloadSpeed: virtv2.ImageStatusSpeed{
//				Avg:          "000",
//				AvgBytes:     "111",
//				Current:      "222",
//				CurrentBytes: "333",
//			},
//			Size: virtv2.ImageStatusSize{
//				Stored:        "AAA",
//				StoredBytes:   "BBB",
//				Unpacked:      "CCC",
//				UnpackedBytes: "DDD",
//			},
//			Format: "qcow2",
//			CDROM:  true,
//			Target: virtv2.ImageStatusTarget{
//				RegistryURL: "dvcr.d8-virtualization.svc/cvi/cvi-example",
//			},
//			Progress: "15%",
//		},
//	}
// }

// func getStatMock(expectedStatus virtv2.ClusterVirtualImageStatus) *StatMock {
//	return &StatMock{
//		GetCDROMFunc: func(pod *corev1.Pod) bool {
//			return expectedStatus.CDROM
//		},
//		GetDownloadSpeedFunc: func(ownerUID types.UID, pod *corev1.Pod) virtv2.ImageStatusSpeed {
//			return expectedStatus.DownloadSpeed
//		},
//		GetFormatFunc: func(pod *corev1.Pod) string {
//			return expectedStatus.Format
//		},
//		GetProgressFunc: func(ownerUID types.UID, pod *corev1.Pod, prevProgress string, opts ...service.GetProgressOption) string {
//			return expectedStatus.Progress
//		},
//		GetSizeFunc: func(pod *corev1.Pod) virtv2.ImageStatusSize {
//			return expectedStatus.Size
//		},
//	}
// }
