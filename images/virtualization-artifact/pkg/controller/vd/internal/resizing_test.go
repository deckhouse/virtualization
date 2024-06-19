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

// func TestResizedHandler_Handle(t *testing.T) {
// 	ctx := context.TODO()
//
// 	t.Run("VirtualDisk with DeletionTimestamp", func(t *testing.T) {
// 		vd := virtv2.VirtualDisk{
// 			ObjectMeta: metav1.ObjectMeta{
// 				DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
// 			},
// 		}
//
// 		handler := NewResizingHandler(nil)
// 		_, err := handler.Handle(ctx, &vd)
// 		require.NoError(t, err)
//
// 		condition := vd.Status.Conditions[0]
// 		require.Equal(t, vdcondition.ResizedType, condition.Type)
// 		require.Equal(t, metav1.ConditionUnknown, condition.Status)
// 		require.Equal(t, "", condition.Reason)
// 	})
//
// 	t.Run("Resize VirtualDisk", func(t *testing.T) {
// 		vd := virtv2.VirtualDisk{
// 			Spec: virtv2.VirtualDiskSpec{
// 				PersistentVolumeClaim: virtv2.VirtualDiskPersistentVolumeClaim{
// 					Size: resource.NewQuantity(1111, resource.BinarySI),
// 				},
// 			},
// 			Status: virtv2.VirtualDiskStatus{
// 				Conditions: []metav1.Condition{
// 					{
// 						Type:   vdcondition.ReadyType,
// 						Status: metav1.ConditionTrue,
// 					},
// 				},
// 			},
// 		}
//
// 		resizer := DiskMock{
// 			GetPersistentVolumeClaimFunc: func(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
// 				return &corev1.PersistentVolumeClaim{
// 					Status: corev1.PersistentVolumeClaimStatus{
// 						Capacity: corev1.ResourceList{
// 							corev1.ResourceStorage: *resource.NewQuantity(2222, resource.BinarySI),
// 						},
// 					},
// 				}, nil
// 			},
// 			ResizeFunc: func(ctx context.Context, newSize resource.Quantity, sup *supplements.Generator) error {
// 				return nil
// 			},
// 		}
//
// 		handler := NewResizedHandler(&resizer)
// 		err := handler.Handle(ctx, &vd)
// 		require.NoError(t, err)
//
// 		condition, ok := getCondition(vdcondition.ResizedType, vd.Status.Conditions)
// 		require.True(t, ok)
// 		require.Equal(t, vdcondition.ResizedType, condition.Type)
// 		require.Equal(t, metav1.ConditionFalse, condition.Status)
// 		require.Equal(t, vdcondition.ResizedReason_InProgress, condition.Reason)
// 	})
//
// 	t.Run("VirtualDisk resized", func(t *testing.T) {
// 		size := resource.NewQuantity(1111, resource.BinarySI)
//
// 		vd := virtv2.VirtualDisk{
// 			Spec: virtv2.VirtualDiskSpec{
// 				PersistentVolumeClaim: virtv2.VirtualDiskPersistentVolumeClaim{
// 					Size: size,
// 				},
// 			},
// 			Status: virtv2.VirtualDiskStatus{
// 				Conditions: []metav1.Condition{
// 					{
// 						Type:   vdcondition.ReadyType,
// 						Status: metav1.ConditionTrue,
// 					},
// 					{
// 						Type:   vdcondition.ResizedType,
// 						Reason: vdcondition.ResizedReason_InProgress,
// 						Status: metav1.ConditionFalse,
// 					},
// 				},
// 			},
// 		}
//
// 		resizer := DiskMock{
// 			GetPersistentVolumeClaimFunc: func(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
// 				return &corev1.PersistentVolumeClaim{
// 					Status: corev1.PersistentVolumeClaimStatus{
// 						Capacity: corev1.ResourceList{
// 							corev1.ResourceStorage: *size,
// 						},
// 					},
// 				}, nil
// 			},
// 		}
//
// 		handler := NewResizedHandler(&resizer)
// 		err := handler.Handle(ctx, &vd)
// 		require.NoError(t, err)
//
// 		condition, ok := getCondition(vdcondition.ResizedType, vd.Status.Conditions)
// 		require.True(t, ok)
// 		require.Equal(t, vdcondition.ResizedType, condition.Type)
// 		require.Equal(t, metav1.ConditionTrue, condition.Status)
// 		require.Equal(t, "", condition.Reason)
// 	})
// }
