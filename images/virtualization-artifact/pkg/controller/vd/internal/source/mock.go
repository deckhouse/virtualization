/*
Copyright 2025 Flant JSC

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

// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package source

import (
	"context"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
)

// Ensure, that ObjectRefVirtualDiskSnapshotDiskServiceMock does implement ObjectRefVirtualDiskSnapshotDiskService.
// If this is not the case, regenerate this file with moq.
var _ ObjectRefVirtualDiskSnapshotDiskService = &ObjectRefVirtualDiskSnapshotDiskServiceMock{}

// ObjectRefVirtualDiskSnapshotDiskServiceMock is a mock implementation of ObjectRefVirtualDiskSnapshotDiskService.
//
//	func TestSomethingThatUsesObjectRefVirtualDiskSnapshotDiskService(t *testing.T) {
//
//		// make and configure a mocked ObjectRefVirtualDiskSnapshotDiskService
//		mockedObjectRefVirtualDiskSnapshotDiskService := &ObjectRefVirtualDiskSnapshotDiskServiceMock{
//			GetCapacityFunc: func(pvc *corev1.PersistentVolumeClaim) string {
//				panic("mock out the GetCapacity method")
//			},
//			GetVirtualDiskSnapshotFunc: func(ctx context.Context, name string, namespace string) (*virtv2.VirtualDiskSnapshot, error) {
//				panic("mock out the GetVirtualDiskSnapshot method")
//			},
//			GetVolumeSnapshotFunc: func(ctx context.Context, name string, namespace string) (*vsv1.VolumeSnapshot, error) {
//				panic("mock out the GetVolumeSnapshot method")
//			},
//			ProtectFunc: func(ctx context.Context, owner client.Object, dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim) error {
//				panic("mock out the Protect method")
//			},
//		}
//
//		// use mockedObjectRefVirtualDiskSnapshotDiskService in code that requires ObjectRefVirtualDiskSnapshotDiskService
//		// and then make assertions.
//
//	}
type ObjectRefVirtualDiskSnapshotDiskServiceMock struct {
	// GetCapacityFunc mocks the GetCapacity method.
	GetCapacityFunc func(pvc *corev1.PersistentVolumeClaim) string

	// GetVirtualDiskSnapshotFunc mocks the GetVirtualDiskSnapshot method.
	GetVirtualDiskSnapshotFunc func(ctx context.Context, name string, namespace string) (*virtv2.VirtualDiskSnapshot, error)

	// GetVolumeSnapshotFunc mocks the GetVolumeSnapshot method.
	GetVolumeSnapshotFunc func(ctx context.Context, name string, namespace string) (*vsv1.VolumeSnapshot, error)

	// ProtectFunc mocks the Protect method.
	ProtectFunc func(ctx context.Context, owner client.Object, dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim) error

	// calls tracks calls to the methods.
	calls struct {
		// GetCapacity holds details about calls to the GetCapacity method.
		GetCapacity []struct {
			// Pvc is the pvc argument value.
			Pvc *corev1.PersistentVolumeClaim
		}
		// GetVirtualDiskSnapshot holds details about calls to the GetVirtualDiskSnapshot method.
		GetVirtualDiskSnapshot []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Name is the name argument value.
			Name string
			// Namespace is the namespace argument value.
			Namespace string
		}
		// GetVolumeSnapshot holds details about calls to the GetVolumeSnapshot method.
		GetVolumeSnapshot []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Name is the name argument value.
			Name string
			// Namespace is the namespace argument value.
			Namespace string
		}
		// Protect holds details about calls to the Protect method.
		Protect []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Owner is the owner argument value.
			Owner client.Object
			// Dv is the dv argument value.
			Dv *cdiv1.DataVolume
			// Pvc is the pvc argument value.
			Pvc *corev1.PersistentVolumeClaim
		}
	}
	lockGetCapacity            sync.RWMutex
	lockGetVirtualDiskSnapshot sync.RWMutex
	lockGetVolumeSnapshot      sync.RWMutex
	lockProtect                sync.RWMutex
}

// GetCapacity calls GetCapacityFunc.
func (mock *ObjectRefVirtualDiskSnapshotDiskServiceMock) GetCapacity(pvc *corev1.PersistentVolumeClaim) string {
	if mock.GetCapacityFunc == nil {
		panic("ObjectRefVirtualDiskSnapshotDiskServiceMock.GetCapacityFunc: method is nil but ObjectRefVirtualDiskSnapshotDiskService.GetCapacity was just called")
	}
	callInfo := struct {
		Pvc *corev1.PersistentVolumeClaim
	}{
		Pvc: pvc,
	}
	mock.lockGetCapacity.Lock()
	mock.calls.GetCapacity = append(mock.calls.GetCapacity, callInfo)
	mock.lockGetCapacity.Unlock()
	return mock.GetCapacityFunc(pvc)
}

// GetCapacityCalls gets all the calls that were made to GetCapacity.
// Check the length with:
//
//	len(mockedObjectRefVirtualDiskSnapshotDiskService.GetCapacityCalls())
func (mock *ObjectRefVirtualDiskSnapshotDiskServiceMock) GetCapacityCalls() []struct {
	Pvc *corev1.PersistentVolumeClaim
} {
	var calls []struct {
		Pvc *corev1.PersistentVolumeClaim
	}
	mock.lockGetCapacity.RLock()
	calls = mock.calls.GetCapacity
	mock.lockGetCapacity.RUnlock()
	return calls
}

// GetVirtualDiskSnapshot calls GetVirtualDiskSnapshotFunc.
func (mock *ObjectRefVirtualDiskSnapshotDiskServiceMock) GetVirtualDiskSnapshot(ctx context.Context, name string, namespace string) (*virtv2.VirtualDiskSnapshot, error) {
	if mock.GetVirtualDiskSnapshotFunc == nil {
		panic("ObjectRefVirtualDiskSnapshotDiskServiceMock.GetVirtualDiskSnapshotFunc: method is nil but ObjectRefVirtualDiskSnapshotDiskService.GetVirtualDiskSnapshot was just called")
	}
	callInfo := struct {
		Ctx       context.Context
		Name      string
		Namespace string
	}{
		Ctx:       ctx,
		Name:      name,
		Namespace: namespace,
	}
	mock.lockGetVirtualDiskSnapshot.Lock()
	mock.calls.GetVirtualDiskSnapshot = append(mock.calls.GetVirtualDiskSnapshot, callInfo)
	mock.lockGetVirtualDiskSnapshot.Unlock()
	return mock.GetVirtualDiskSnapshotFunc(ctx, name, namespace)
}

// GetVirtualDiskSnapshotCalls gets all the calls that were made to GetVirtualDiskSnapshot.
// Check the length with:
//
//	len(mockedObjectRefVirtualDiskSnapshotDiskService.GetVirtualDiskSnapshotCalls())
func (mock *ObjectRefVirtualDiskSnapshotDiskServiceMock) GetVirtualDiskSnapshotCalls() []struct {
	Ctx       context.Context
	Name      string
	Namespace string
} {
	var calls []struct {
		Ctx       context.Context
		Name      string
		Namespace string
	}
	mock.lockGetVirtualDiskSnapshot.RLock()
	calls = mock.calls.GetVirtualDiskSnapshot
	mock.lockGetVirtualDiskSnapshot.RUnlock()
	return calls
}

// GetVolumeSnapshot calls GetVolumeSnapshotFunc.
func (mock *ObjectRefVirtualDiskSnapshotDiskServiceMock) GetVolumeSnapshot(ctx context.Context, name string, namespace string) (*vsv1.VolumeSnapshot, error) {
	if mock.GetVolumeSnapshotFunc == nil {
		panic("ObjectRefVirtualDiskSnapshotDiskServiceMock.GetVolumeSnapshotFunc: method is nil but ObjectRefVirtualDiskSnapshotDiskService.GetVolumeSnapshot was just called")
	}
	callInfo := struct {
		Ctx       context.Context
		Name      string
		Namespace string
	}{
		Ctx:       ctx,
		Name:      name,
		Namespace: namespace,
	}
	mock.lockGetVolumeSnapshot.Lock()
	mock.calls.GetVolumeSnapshot = append(mock.calls.GetVolumeSnapshot, callInfo)
	mock.lockGetVolumeSnapshot.Unlock()
	return mock.GetVolumeSnapshotFunc(ctx, name, namespace)
}

// GetVolumeSnapshotCalls gets all the calls that were made to GetVolumeSnapshot.
// Check the length with:
//
//	len(mockedObjectRefVirtualDiskSnapshotDiskService.GetVolumeSnapshotCalls())
func (mock *ObjectRefVirtualDiskSnapshotDiskServiceMock) GetVolumeSnapshotCalls() []struct {
	Ctx       context.Context
	Name      string
	Namespace string
} {
	var calls []struct {
		Ctx       context.Context
		Name      string
		Namespace string
	}
	mock.lockGetVolumeSnapshot.RLock()
	calls = mock.calls.GetVolumeSnapshot
	mock.lockGetVolumeSnapshot.RUnlock()
	return calls
}

// Protect calls ProtectFunc.
func (mock *ObjectRefVirtualDiskSnapshotDiskServiceMock) Protect(ctx context.Context, owner client.Object, dv *cdiv1.DataVolume, pvc *corev1.PersistentVolumeClaim) error {
	if mock.ProtectFunc == nil {
		panic("ObjectRefVirtualDiskSnapshotDiskServiceMock.ProtectFunc: method is nil but ObjectRefVirtualDiskSnapshotDiskService.Protect was just called")
	}
	callInfo := struct {
		Ctx   context.Context
		Owner client.Object
		Dv    *cdiv1.DataVolume
		Pvc   *corev1.PersistentVolumeClaim
	}{
		Ctx:   ctx,
		Owner: owner,
		Dv:    dv,
		Pvc:   pvc,
	}
	mock.lockProtect.Lock()
	mock.calls.Protect = append(mock.calls.Protect, callInfo)
	mock.lockProtect.Unlock()
	return mock.ProtectFunc(ctx, owner, dv, pvc)
}

// ProtectCalls gets all the calls that were made to Protect.
// Check the length with:
//
//	len(mockedObjectRefVirtualDiskSnapshotDiskService.ProtectCalls())
func (mock *ObjectRefVirtualDiskSnapshotDiskServiceMock) ProtectCalls() []struct {
	Ctx   context.Context
	Owner client.Object
	Dv    *cdiv1.DataVolume
	Pvc   *corev1.PersistentVolumeClaim
} {
	var calls []struct {
		Ctx   context.Context
		Owner client.Object
		Dv    *cdiv1.DataVolume
		Pvc   *corev1.PersistentVolumeClaim
	}
	mock.lockProtect.RLock()
	calls = mock.calls.Protect
	mock.lockProtect.RUnlock()
	return calls
}
