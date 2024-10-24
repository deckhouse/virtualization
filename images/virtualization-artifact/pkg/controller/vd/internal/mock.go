// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package internal

import (
	"context"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sync"
)

// Ensure, that HandlerMock does implement Handler.
// If this is not the case, regenerate this file with moq.
var _ Handler = &HandlerMock{}

// HandlerMock is a mock implementation of Handler.
//
//	func TestSomethingThatUsesHandler(t *testing.T) {
//
//		// make and configure a mocked Handler
//		mockedHandler := &HandlerMock{
//			CleanUpFunc: func(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
//				panic("mock out the CleanUp method")
//			},
//			NameFunc: func() string {
//				panic("mock out the Name method")
//			},
//			SyncFunc: func(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
//				panic("mock out the Sync method")
//			},
//			ValidateFunc: func(ctx context.Context, vd *virtv2.VirtualDisk) error {
//				panic("mock out the Validate method")
//			},
//		}
//
//		// use mockedHandler in code that requires Handler
//		// and then make assertions.
//
//	}
type HandlerMock struct {
	// CleanUpFunc mocks the CleanUp method.
	CleanUpFunc func(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error)

	// NameFunc mocks the Name method.
	NameFunc func() string

	// SyncFunc mocks the Sync method.
	SyncFunc func(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error)

	// ValidateFunc mocks the Validate method.
	ValidateFunc func(ctx context.Context, vd *virtv2.VirtualDisk) error

	// calls tracks calls to the methods.
	calls struct {
		// CleanUp holds details about calls to the CleanUp method.
		CleanUp []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Vd is the vd argument value.
			Vd *virtv2.VirtualDisk
		}
		// Name holds details about calls to the Name method.
		Name []struct {
		}
		// Sync holds details about calls to the Sync method.
		Sync []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Vd is the vd argument value.
			Vd *virtv2.VirtualDisk
		}
		// Validate holds details about calls to the Validate method.
		Validate []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Vd is the vd argument value.
			Vd *virtv2.VirtualDisk
		}
	}
	lockCleanUp  sync.RWMutex
	lockName     sync.RWMutex
	lockSync     sync.RWMutex
	lockValidate sync.RWMutex
}

// CleanUp calls CleanUpFunc.
func (mock *HandlerMock) CleanUp(ctx context.Context, vd *virtv2.VirtualDisk) (bool, error) {
	if mock.CleanUpFunc == nil {
		panic("HandlerMock.CleanUpFunc: method is nil but Handler.CleanUp was just called")
	}
	callInfo := struct {
		Ctx context.Context
		Vd  *virtv2.VirtualDisk
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
//	len(mockedHandler.CleanUpCalls())
func (mock *HandlerMock) CleanUpCalls() []struct {
	Ctx context.Context
	Vd  *virtv2.VirtualDisk
} {
	var calls []struct {
		Ctx context.Context
		Vd  *virtv2.VirtualDisk
	}
	mock.lockCleanUp.RLock()
	calls = mock.calls.CleanUp
	mock.lockCleanUp.RUnlock()
	return calls
}

// Name calls NameFunc.
func (mock *HandlerMock) Name() string {
	if mock.NameFunc == nil {
		panic("HandlerMock.NameFunc: method is nil but Handler.Name was just called")
	}
	callInfo := struct {
	}{}
	mock.lockName.Lock()
	mock.calls.Name = append(mock.calls.Name, callInfo)
	mock.lockName.Unlock()
	return mock.NameFunc()
}

// NameCalls gets all the calls that were made to Name.
// Check the length with:
//
//	len(mockedHandler.NameCalls())
func (mock *HandlerMock) NameCalls() []struct {
} {
	var calls []struct {
	}
	mock.lockName.RLock()
	calls = mock.calls.Name
	mock.lockName.RUnlock()
	return calls
}

// Sync calls SyncFunc.
func (mock *HandlerMock) Sync(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	if mock.SyncFunc == nil {
		panic("HandlerMock.SyncFunc: method is nil but Handler.Sync was just called")
	}
	callInfo := struct {
		Ctx context.Context
		Vd  *virtv2.VirtualDisk
	}{
		Ctx: ctx,
		Vd:  vd,
	}
	mock.lockSync.Lock()
	mock.calls.Sync = append(mock.calls.Sync, callInfo)
	mock.lockSync.Unlock()
	return mock.SyncFunc(ctx, vd)
}

// SyncCalls gets all the calls that were made to Sync.
// Check the length with:
//
//	len(mockedHandler.SyncCalls())
func (mock *HandlerMock) SyncCalls() []struct {
	Ctx context.Context
	Vd  *virtv2.VirtualDisk
} {
	var calls []struct {
		Ctx context.Context
		Vd  *virtv2.VirtualDisk
	}
	mock.lockSync.RLock()
	calls = mock.calls.Sync
	mock.lockSync.RUnlock()
	return calls
}

// Validate calls ValidateFunc.
func (mock *HandlerMock) Validate(ctx context.Context, vd *virtv2.VirtualDisk) error {
	if mock.ValidateFunc == nil {
		panic("HandlerMock.ValidateFunc: method is nil but Handler.Validate was just called")
	}
	callInfo := struct {
		Ctx context.Context
		Vd  *virtv2.VirtualDisk
	}{
		Ctx: ctx,
		Vd:  vd,
	}
	mock.lockValidate.Lock()
	mock.calls.Validate = append(mock.calls.Validate, callInfo)
	mock.lockValidate.Unlock()
	return mock.ValidateFunc(ctx, vd)
}

// ValidateCalls gets all the calls that were made to Validate.
// Check the length with:
//
//	len(mockedHandler.ValidateCalls())
func (mock *HandlerMock) ValidateCalls() []struct {
	Ctx context.Context
	Vd  *virtv2.VirtualDisk
} {
	var calls []struct {
		Ctx context.Context
		Vd  *virtv2.VirtualDisk
	}
	mock.lockValidate.RLock()
	calls = mock.calls.Validate
	mock.lockValidate.RUnlock()
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
//			GetFunc: func(dsType virtv2.DataSourceType) (source.Handler, bool) {
//				panic("mock out the Get method")
//			},
//		}
//
//		// use mockedSources in code that requires Sources
//		// and then make assertions.
//
//	}
type SourcesMock struct {
	// GetFunc mocks the Get method.
	GetFunc func(dsType virtv2.DataSourceType) (source.Handler, bool)

	// calls tracks calls to the methods.
	calls struct {
		// Get holds details about calls to the Get method.
		Get []struct {
			// DsType is the dsType argument value.
			DsType virtv2.DataSourceType
		}
	}
	lockGet sync.RWMutex
}

// Get calls GetFunc.
func (mock *SourcesMock) Get(dsType virtv2.DataSourceType) (source.Handler, bool) {
	if mock.GetFunc == nil {
		panic("SourcesMock.GetFunc: method is nil but Sources.Get was just called")
	}
	callInfo := struct {
		DsType virtv2.DataSourceType
	}{
		DsType: dsType,
	}
	mock.lockGet.Lock()
	mock.calls.Get = append(mock.calls.Get, callInfo)
	mock.lockGet.Unlock()
	return mock.GetFunc(dsType)
}

// GetCalls gets all the calls that were made to Get.
// Check the length with:
//
//	len(mockedSources.GetCalls())
func (mock *SourcesMock) GetCalls() []struct {
	DsType virtv2.DataSourceType
} {
	var calls []struct {
		DsType virtv2.DataSourceType
	}
	mock.lockGet.RLock()
	calls = mock.calls.Get
	mock.lockGet.RUnlock()
	return calls
}

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
//			ResizeFunc: func(ctx context.Context, pvc *corev1.PersistentVolumeClaim, newSize resource.Quantity) error {
//				panic("mock out the Resize method")
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

	// ResizeFunc mocks the Resize method.
	ResizeFunc func(ctx context.Context, pvc *corev1.PersistentVolumeClaim, newSize resource.Quantity) error

	// calls tracks calls to the methods.
	calls struct {
		// GetPersistentVolumeClaim holds details about calls to the GetPersistentVolumeClaim method.
		GetPersistentVolumeClaim []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Sup is the sup argument value.
			Sup *supplements.Generator
		}
		// Resize holds details about calls to the Resize method.
		Resize []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Pvc is the pvc argument value.
			Pvc *corev1.PersistentVolumeClaim
			// NewSize is the newSize argument value.
			NewSize resource.Quantity
		}
	}
	lockGetPersistentVolumeClaim sync.RWMutex
	lockResize                   sync.RWMutex
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

// Resize calls ResizeFunc.
func (mock *DiskServiceMock) Resize(ctx context.Context, pvc *corev1.PersistentVolumeClaim, newSize resource.Quantity) error {
	if mock.ResizeFunc == nil {
		panic("DiskServiceMock.ResizeFunc: method is nil but DiskService.Resize was just called")
	}
	callInfo := struct {
		Ctx     context.Context
		Pvc     *corev1.PersistentVolumeClaim
		NewSize resource.Quantity
	}{
		Ctx:     ctx,
		Pvc:     pvc,
		NewSize: newSize,
	}
	mock.lockResize.Lock()
	mock.calls.Resize = append(mock.calls.Resize, callInfo)
	mock.lockResize.Unlock()
	return mock.ResizeFunc(ctx, pvc, newSize)
}

// ResizeCalls gets all the calls that were made to Resize.
// Check the length with:
//
//	len(mockedDiskService.ResizeCalls())
func (mock *DiskServiceMock) ResizeCalls() []struct {
	Ctx     context.Context
	Pvc     *corev1.PersistentVolumeClaim
	NewSize resource.Quantity
} {
	var calls []struct {
		Ctx     context.Context
		Pvc     *corev1.PersistentVolumeClaim
		NewSize resource.Quantity
	}
	mock.lockResize.RLock()
	calls = mock.calls.Resize
	mock.lockResize.RUnlock()
	return calls
}
