// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package internal

import (
	"context"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"sync"
)

// Ensure, that RestorerMock does implement Restorer.
// If this is not the case, regenerate this file with moq.
var _ Restorer = &RestorerMock{}

// RestorerMock is a mock implementation of Restorer.
//
//	func TestSomethingThatUsesRestorer(t *testing.T) {
//
//		// make and configure a mocked Restorer
//		mockedRestorer := &RestorerMock{
//			RestoreProvisionerFunc: func(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error) {
//				panic("mock out the RestoreProvisioner method")
//			},
//			RestoreVirtualMachineFunc: func(ctx context.Context, secret *corev1.Secret) (*virtv2.VirtualMachine, error) {
//				panic("mock out the RestoreVirtualMachine method")
//			},
//			RestoreVirtualMachineBlockDeviceAttachmentsFunc: func(ctx context.Context, secret *corev1.Secret) ([]*virtv2.VirtualMachineBlockDeviceAttachment, error) {
//				panic("mock out the RestoreVirtualMachineBlockDeviceAttachments method")
//			},
//			RestoreVirtualMachineIPAddressFunc: func(ctx context.Context, secret *corev1.Secret) (*virtv2.VirtualMachineIPAddress, error) {
//				panic("mock out the RestoreVirtualMachineIPAddress method")
//			},
//		}
//
//		// use mockedRestorer in code that requires Restorer
//		// and then make assertions.
//
//	}
type RestorerMock struct {
	// RestoreProvisionerFunc mocks the RestoreProvisioner method.
	RestoreProvisionerFunc func(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error)

	// RestoreVirtualMachineFunc mocks the RestoreVirtualMachine method.
	RestoreVirtualMachineFunc func(ctx context.Context, secret *corev1.Secret) (*virtv2.VirtualMachine, error)

	// RestoreVirtualMachineBlockDeviceAttachmentsFunc mocks the RestoreVirtualMachineBlockDeviceAttachments method.
	RestoreVirtualMachineBlockDeviceAttachmentsFunc func(ctx context.Context, secret *corev1.Secret) ([]*virtv2.VirtualMachineBlockDeviceAttachment, error)

	// RestoreVirtualMachineIPAddressFunc mocks the RestoreVirtualMachineIPAddress method.
	RestoreVirtualMachineIPAddressFunc func(ctx context.Context, secret *corev1.Secret) (*virtv2.VirtualMachineIPAddress, error)

	// calls tracks calls to the methods.
	calls struct {
		// RestoreProvisioner holds details about calls to the RestoreProvisioner method.
		RestoreProvisioner []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Secret is the secret argument value.
			Secret *corev1.Secret
		}
		// RestoreVirtualMachine holds details about calls to the RestoreVirtualMachine method.
		RestoreVirtualMachine []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Secret is the secret argument value.
			Secret *corev1.Secret
		}
		// RestoreVirtualMachineBlockDeviceAttachments holds details about calls to the RestoreVirtualMachineBlockDeviceAttachments method.
		RestoreVirtualMachineBlockDeviceAttachments []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Secret is the secret argument value.
			Secret *corev1.Secret
		}
		// RestoreVirtualMachineIPAddress holds details about calls to the RestoreVirtualMachineIPAddress method.
		RestoreVirtualMachineIPAddress []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Secret is the secret argument value.
			Secret *corev1.Secret
		}
	}
	lockRestoreProvisioner                          sync.RWMutex
	lockRestoreVirtualMachine                       sync.RWMutex
	lockRestoreVirtualMachineBlockDeviceAttachments sync.RWMutex
	lockRestoreVirtualMachineIPAddress              sync.RWMutex
}

// RestoreProvisioner calls RestoreProvisionerFunc.
func (mock *RestorerMock) RestoreProvisioner(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error) {
	if mock.RestoreProvisionerFunc == nil {
		panic("RestorerMock.RestoreProvisionerFunc: method is nil but Restorer.RestoreProvisioner was just called")
	}
	callInfo := struct {
		Ctx    context.Context
		Secret *corev1.Secret
	}{
		Ctx:    ctx,
		Secret: secret,
	}
	mock.lockRestoreProvisioner.Lock()
	mock.calls.RestoreProvisioner = append(mock.calls.RestoreProvisioner, callInfo)
	mock.lockRestoreProvisioner.Unlock()
	return mock.RestoreProvisionerFunc(ctx, secret)
}

// RestoreProvisionerCalls gets all the calls that were made to RestoreProvisioner.
// Check the length with:
//
//	len(mockedRestorer.RestoreProvisionerCalls())
func (mock *RestorerMock) RestoreProvisionerCalls() []struct {
	Ctx    context.Context
	Secret *corev1.Secret
} {
	var calls []struct {
		Ctx    context.Context
		Secret *corev1.Secret
	}
	mock.lockRestoreProvisioner.RLock()
	calls = mock.calls.RestoreProvisioner
	mock.lockRestoreProvisioner.RUnlock()
	return calls
}

// RestoreVirtualMachine calls RestoreVirtualMachineFunc.
func (mock *RestorerMock) RestoreVirtualMachine(ctx context.Context, secret *corev1.Secret) (*virtv2.VirtualMachine, error) {
	if mock.RestoreVirtualMachineFunc == nil {
		panic("RestorerMock.RestoreVirtualMachineFunc: method is nil but Restorer.RestoreVirtualMachine was just called")
	}
	callInfo := struct {
		Ctx    context.Context
		Secret *corev1.Secret
	}{
		Ctx:    ctx,
		Secret: secret,
	}
	mock.lockRestoreVirtualMachine.Lock()
	mock.calls.RestoreVirtualMachine = append(mock.calls.RestoreVirtualMachine, callInfo)
	mock.lockRestoreVirtualMachine.Unlock()
	return mock.RestoreVirtualMachineFunc(ctx, secret)
}

// RestoreVirtualMachineCalls gets all the calls that were made to RestoreVirtualMachine.
// Check the length with:
//
//	len(mockedRestorer.RestoreVirtualMachineCalls())
func (mock *RestorerMock) RestoreVirtualMachineCalls() []struct {
	Ctx    context.Context
	Secret *corev1.Secret
} {
	var calls []struct {
		Ctx    context.Context
		Secret *corev1.Secret
	}
	mock.lockRestoreVirtualMachine.RLock()
	calls = mock.calls.RestoreVirtualMachine
	mock.lockRestoreVirtualMachine.RUnlock()
	return calls
}

// RestoreVirtualMachineBlockDeviceAttachments calls RestoreVirtualMachineBlockDeviceAttachmentsFunc.
func (mock *RestorerMock) RestoreVirtualMachineBlockDeviceAttachments(ctx context.Context, secret *corev1.Secret) ([]*virtv2.VirtualMachineBlockDeviceAttachment, error) {
	if mock.RestoreVirtualMachineBlockDeviceAttachmentsFunc == nil {
		panic("RestorerMock.RestoreVirtualMachineBlockDeviceAttachmentsFunc: method is nil but Restorer.RestoreVirtualMachineBlockDeviceAttachments was just called")
	}
	callInfo := struct {
		Ctx    context.Context
		Secret *corev1.Secret
	}{
		Ctx:    ctx,
		Secret: secret,
	}
	mock.lockRestoreVirtualMachineBlockDeviceAttachments.Lock()
	mock.calls.RestoreVirtualMachineBlockDeviceAttachments = append(mock.calls.RestoreVirtualMachineBlockDeviceAttachments, callInfo)
	mock.lockRestoreVirtualMachineBlockDeviceAttachments.Unlock()
	return mock.RestoreVirtualMachineBlockDeviceAttachmentsFunc(ctx, secret)
}

// RestoreVirtualMachineBlockDeviceAttachmentsCalls gets all the calls that were made to RestoreVirtualMachineBlockDeviceAttachments.
// Check the length with:
//
//	len(mockedRestorer.RestoreVirtualMachineBlockDeviceAttachmentsCalls())
func (mock *RestorerMock) RestoreVirtualMachineBlockDeviceAttachmentsCalls() []struct {
	Ctx    context.Context
	Secret *corev1.Secret
} {
	var calls []struct {
		Ctx    context.Context
		Secret *corev1.Secret
	}
	mock.lockRestoreVirtualMachineBlockDeviceAttachments.RLock()
	calls = mock.calls.RestoreVirtualMachineBlockDeviceAttachments
	mock.lockRestoreVirtualMachineBlockDeviceAttachments.RUnlock()
	return calls
}

// RestoreVirtualMachineIPAddress calls RestoreVirtualMachineIPAddressFunc.
func (mock *RestorerMock) RestoreVirtualMachineIPAddress(ctx context.Context, secret *corev1.Secret) (*virtv2.VirtualMachineIPAddress, error) {
	if mock.RestoreVirtualMachineIPAddressFunc == nil {
		panic("RestorerMock.RestoreVirtualMachineIPAddressFunc: method is nil but Restorer.RestoreVirtualMachineIPAddress was just called")
	}
	callInfo := struct {
		Ctx    context.Context
		Secret *corev1.Secret
	}{
		Ctx:    ctx,
		Secret: secret,
	}
	mock.lockRestoreVirtualMachineIPAddress.Lock()
	mock.calls.RestoreVirtualMachineIPAddress = append(mock.calls.RestoreVirtualMachineIPAddress, callInfo)
	mock.lockRestoreVirtualMachineIPAddress.Unlock()
	return mock.RestoreVirtualMachineIPAddressFunc(ctx, secret)
}

// RestoreVirtualMachineIPAddressCalls gets all the calls that were made to RestoreVirtualMachineIPAddress.
// Check the length with:
//
//	len(mockedRestorer.RestoreVirtualMachineIPAddressCalls())
func (mock *RestorerMock) RestoreVirtualMachineIPAddressCalls() []struct {
	Ctx    context.Context
	Secret *corev1.Secret
} {
	var calls []struct {
		Ctx    context.Context
		Secret *corev1.Secret
	}
	mock.lockRestoreVirtualMachineIPAddress.RLock()
	calls = mock.calls.RestoreVirtualMachineIPAddress
	mock.lockRestoreVirtualMachineIPAddress.RUnlock()
	return calls
}
