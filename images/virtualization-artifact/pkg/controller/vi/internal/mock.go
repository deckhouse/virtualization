// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package internal

import (
	"context"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vi/internal/source"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	storev1 "k8s.io/api/storage/v1"
	"sync"
)

// Ensure, that DiskServiceMock does implement DiskService.
// If this is not the case, regenerate this file with moq.
var _ DiskService = &DiskServiceMock{}

// DiskServiceMock is a mock implementation of DiskService.
//
//	func TestSomethingThatUsesDiskService(t *testing.T) {
//
//		// make and configure a mocked DiskService
//		mockedDiskService := &DiskServiceMock{
//			GetPersistentVolumeClaimFunc: func(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
//				panic("mock out the GetPersistentVolumeClaim method")
//			},
//			GetStorageClassFunc: func(ctx context.Context, storageClassName *string) (*storev1.StorageClass, error) {
//				panic("mock out the GetStorageClass method")
//			},
//		}
//
//		// use mockedDiskService in code that requires DiskService
//		// and then make assertions.
//
//	}
type DiskServiceMock struct {
	// GetPersistentVolumeClaimFunc mocks the GetPersistentVolumeClaim method.
	GetPersistentVolumeClaimFunc func(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error)

	// GetStorageClassFunc mocks the GetStorageClass method.
	GetStorageClassFunc func(ctx context.Context, storageClassName *string) (*storev1.StorageClass, error)

	// calls tracks calls to the methods.
	calls struct {
		// GetPersistentVolumeClaim holds details about calls to the GetPersistentVolumeClaim method.
		GetPersistentVolumeClaim []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Sup is the sup argument value.
			Sup *supplements.Generator
		}
		// GetStorageClass holds details about calls to the GetStorageClass method.
		GetStorageClass []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// StorageClassName is the storageClassName argument value.
			StorageClassName *string
		}
	}
	lockGetPersistentVolumeClaim sync.RWMutex
	lockGetStorageClass          sync.RWMutex
}

// GetPersistentVolumeClaim calls GetPersistentVolumeClaimFunc.
func (mock *DiskServiceMock) GetPersistentVolumeClaim(ctx context.Context, sup *supplements.Generator) (*corev1.PersistentVolumeClaim, error) {
	if mock.GetPersistentVolumeClaimFunc == nil {
		panic("DiskServiceMock.GetPersistentVolumeClaimFunc: method is nil but DiskService.GetPersistentVolumeClaim was just called")
	}
	callInfo := struct {
		Ctx context.Context
		Sup *supplements.Generator
	}{
		Ctx: ctx,
		Sup: sup,
	}
	mock.lockGetPersistentVolumeClaim.Lock()
	mock.calls.GetPersistentVolumeClaim = append(mock.calls.GetPersistentVolumeClaim, callInfo)
	mock.lockGetPersistentVolumeClaim.Unlock()
	return mock.GetPersistentVolumeClaimFunc(ctx, sup)
}

// GetPersistentVolumeClaimCalls gets all the calls that were made to GetPersistentVolumeClaim.
// Check the length with:
//
//	len(mockedDiskService.GetPersistentVolumeClaimCalls())
func (mock *DiskServiceMock) GetPersistentVolumeClaimCalls() []struct {
	Ctx context.Context
	Sup *supplements.Generator
} {
	var calls []struct {
		Ctx context.Context
		Sup *supplements.Generator
	}
	mock.lockGetPersistentVolumeClaim.RLock()
	calls = mock.calls.GetPersistentVolumeClaim
	mock.lockGetPersistentVolumeClaim.RUnlock()
	return calls
}

// GetStorageClass calls GetStorageClassFunc.
func (mock *DiskServiceMock) GetStorageClass(ctx context.Context, storageClassName *string) (*storev1.StorageClass, error) {
	if mock.GetStorageClassFunc == nil {
		panic("DiskServiceMock.GetStorageClassFunc: method is nil but DiskService.GetStorageClass was just called")
	}
	callInfo := struct {
		Ctx              context.Context
		StorageClassName *string
	}{
		Ctx:              ctx,
		StorageClassName: storageClassName,
	}
	mock.lockGetStorageClass.Lock()
	mock.calls.GetStorageClass = append(mock.calls.GetStorageClass, callInfo)
	mock.lockGetStorageClass.Unlock()
	return mock.GetStorageClassFunc(ctx, storageClassName)
}

// GetStorageClassCalls gets all the calls that were made to GetStorageClass.
// Check the length with:
//
//	len(mockedDiskService.GetStorageClassCalls())
func (mock *DiskServiceMock) GetStorageClassCalls() []struct {
	Ctx              context.Context
	StorageClassName *string
} {
	var calls []struct {
		Ctx              context.Context
		StorageClassName *string
	}
	mock.lockGetStorageClass.RLock()
	calls = mock.calls.GetStorageClass
	mock.lockGetStorageClass.RUnlock()
	return calls
}

// Ensure, that SourcesMock does implement Sources.
// If this is not the case, regenerate this file with moq.
var _ Sources = &SourcesMock{}

// SourcesMock is a mock implementation of Sources.
//
//	func TestSomethingThatUsesSources(t *testing.T) {
//
//		// make and configure a mocked Sources
//		mockedSources := &SourcesMock{
//			ChangedFunc: func(ctx context.Context, vi *virtv2.VirtualImage) bool {
//				panic("mock out the Changed method")
//			},
//			CleanUpFunc: func(ctx context.Context, vd *virtv2.VirtualImage) (bool, error) {
//				panic("mock out the CleanUp method")
//			},
//			ForFunc: func(dsType virtv2.DataSourceType) (source.Handler, bool) {
//				panic("mock out the For method")
//			},
//		}
//
//		// use mockedSources in code that requires Sources
//		// and then make assertions.
//
//	}
type SourcesMock struct {
	// ChangedFunc mocks the Changed method.
	ChangedFunc func(ctx context.Context, vi *virtv2.VirtualImage) bool

	// CleanUpFunc mocks the CleanUp method.
	CleanUpFunc func(ctx context.Context, vd *virtv2.VirtualImage) (bool, error)

	// ForFunc mocks the For method.
	ForFunc func(dsType virtv2.DataSourceType) (source.Handler, bool)

	// calls tracks calls to the methods.
	calls struct {
		// Changed holds details about calls to the Changed method.
		Changed []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Vi is the vi argument value.
			Vi *virtv2.VirtualImage
		}
		// CleanUp holds details about calls to the CleanUp method.
		CleanUp []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Vd is the vd argument value.
			Vd *virtv2.VirtualImage
		}
		// For holds details about calls to the For method.
		For []struct {
			// DsType is the dsType argument value.
			DsType virtv2.DataSourceType
		}
	}
	lockChanged sync.RWMutex
	lockCleanUp sync.RWMutex
	lockFor     sync.RWMutex
}

// Changed calls ChangedFunc.
func (mock *SourcesMock) Changed(ctx context.Context, vi *virtv2.VirtualImage) bool {
	if mock.ChangedFunc == nil {
		panic("SourcesMock.ChangedFunc: method is nil but Sources.Changed was just called")
	}
	callInfo := struct {
		Ctx context.Context
		Vi  *virtv2.VirtualImage
	}{
		Ctx: ctx,
		Vi:  vi,
	}
	mock.lockChanged.Lock()
	mock.calls.Changed = append(mock.calls.Changed, callInfo)
	mock.lockChanged.Unlock()
	return mock.ChangedFunc(ctx, vi)
}

// ChangedCalls gets all the calls that were made to Changed.
// Check the length with:
//
//	len(mockedSources.ChangedCalls())
func (mock *SourcesMock) ChangedCalls() []struct {
	Ctx context.Context
	Vi  *virtv2.VirtualImage
} {
	var calls []struct {
		Ctx context.Context
		Vi  *virtv2.VirtualImage
	}
	mock.lockChanged.RLock()
	calls = mock.calls.Changed
	mock.lockChanged.RUnlock()
	return calls
}

// CleanUp calls CleanUpFunc.
func (mock *SourcesMock) CleanUp(ctx context.Context, vd *virtv2.VirtualImage) (bool, error) {
	if mock.CleanUpFunc == nil {
		panic("SourcesMock.CleanUpFunc: method is nil but Sources.CleanUp was just called")
	}
	callInfo := struct {
		Ctx context.Context
		Vd  *virtv2.VirtualImage
	}{
		Ctx: ctx,
		Vd:  vd,
	}
	mock.lockCleanUp.Lock()
	mock.calls.CleanUp = append(mock.calls.CleanUp, callInfo)
	mock.lockCleanUp.Unlock()
	return mock.CleanUpFunc(ctx, vd)
}

// CleanUpCalls gets all the calls that were made to CleanUp.
// Check the length with:
//
//	len(mockedSources.CleanUpCalls())
func (mock *SourcesMock) CleanUpCalls() []struct {
	Ctx context.Context
	Vd  *virtv2.VirtualImage
} {
	var calls []struct {
		Ctx context.Context
		Vd  *virtv2.VirtualImage
	}
	mock.lockCleanUp.RLock()
	calls = mock.calls.CleanUp
	mock.lockCleanUp.RUnlock()
	return calls
}

// For calls ForFunc.
func (mock *SourcesMock) For(dsType virtv2.DataSourceType) (source.Handler, bool) {
	if mock.ForFunc == nil {
		panic("SourcesMock.ForFunc: method is nil but Sources.For was just called")
	}
	callInfo := struct {
		DsType virtv2.DataSourceType
	}{
		DsType: dsType,
	}
	mock.lockFor.Lock()
	mock.calls.For = append(mock.calls.For, callInfo)
	mock.lockFor.Unlock()
	return mock.ForFunc(dsType)
}

// ForCalls gets all the calls that were made to For.
// Check the length with:
//
//	len(mockedSources.ForCalls())
func (mock *SourcesMock) ForCalls() []struct {
	DsType virtv2.DataSourceType
} {
	var calls []struct {
		DsType virtv2.DataSourceType
	}
	mock.lockFor.RLock()
	calls = mock.calls.For
	mock.lockFor.RUnlock()
	return calls
}
