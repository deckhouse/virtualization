// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package internal

import (
	"context"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	"sync"
)

// Ensure, that EventRecorderMock does implement EventRecorder.
// If this is not the case, regenerate this file with moq.
var _ EventRecorder = &EventRecorderMock{}

// EventRecorderMock is a mock implementation of EventRecorder.
//
//	func TestSomethingThatUsesEventRecorder(t *testing.T) {
//
//		// make and configure a mocked EventRecorder
//		mockedEventRecorder := &EventRecorderMock{
//			AnnotatedEventfFunc: func(object runtime.Object, annotations map[string]string, eventtype string, reason string, messageFmt string, args ...interface{})  {
//				panic("mock out the AnnotatedEventf method")
//			},
//			EventFunc: func(object runtime.Object, eventtype string, reason string, message string)  {
//				panic("mock out the Event method")
//			},
//			EventfFunc: func(object runtime.Object, eventtype string, reason string, messageFmt string, args ...interface{})  {
//				panic("mock out the Eventf method")
//			},
//		}
//
//		// use mockedEventRecorder in code that requires EventRecorder
//		// and then make assertions.
//
//	}
type EventRecorderMock struct {
	// AnnotatedEventfFunc mocks the AnnotatedEventf method.
	AnnotatedEventfFunc func(object runtime.Object, annotations map[string]string, eventtype string, reason string, messageFmt string, args ...interface{})

	// EventFunc mocks the Event method.
	EventFunc func(object runtime.Object, eventtype string, reason string, message string)

	// EventfFunc mocks the Eventf method.
	EventfFunc func(object runtime.Object, eventtype string, reason string, messageFmt string, args ...interface{})

	// calls tracks calls to the methods.
	calls struct {
		// AnnotatedEventf holds details about calls to the AnnotatedEventf method.
		AnnotatedEventf []struct {
			// Object is the object argument value.
			Object runtime.Object
			// Annotations is the annotations argument value.
			Annotations map[string]string
			// Eventtype is the eventtype argument value.
			Eventtype string
			// Reason is the reason argument value.
			Reason string
			// MessageFmt is the messageFmt argument value.
			MessageFmt string
			// Args is the args argument value.
			Args []interface{}
		}
		// Event holds details about calls to the Event method.
		Event []struct {
			// Object is the object argument value.
			Object runtime.Object
			// Eventtype is the eventtype argument value.
			Eventtype string
			// Reason is the reason argument value.
			Reason string
			// Message is the message argument value.
			Message string
		}
		// Eventf holds details about calls to the Eventf method.
		Eventf []struct {
			// Object is the object argument value.
			Object runtime.Object
			// Eventtype is the eventtype argument value.
			Eventtype string
			// Reason is the reason argument value.
			Reason string
			// MessageFmt is the messageFmt argument value.
			MessageFmt string
			// Args is the args argument value.
			Args []interface{}
		}
	}
	lockAnnotatedEventf sync.RWMutex
	lockEvent           sync.RWMutex
	lockEventf          sync.RWMutex
}

// AnnotatedEventf calls AnnotatedEventfFunc.
func (mock *EventRecorderMock) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype string, reason string, messageFmt string, args ...interface{}) {
	if mock.AnnotatedEventfFunc == nil {
		panic("EventRecorderMock.AnnotatedEventfFunc: method is nil but EventRecorder.AnnotatedEventf was just called")
	}
	callInfo := struct {
		Object      runtime.Object
		Annotations map[string]string
		Eventtype   string
		Reason      string
		MessageFmt  string
		Args        []interface{}
	}{
		Object:      object,
		Annotations: annotations,
		Eventtype:   eventtype,
		Reason:      reason,
		MessageFmt:  messageFmt,
		Args:        args,
	}
	mock.lockAnnotatedEventf.Lock()
	mock.calls.AnnotatedEventf = append(mock.calls.AnnotatedEventf, callInfo)
	mock.lockAnnotatedEventf.Unlock()
	mock.AnnotatedEventfFunc(object, annotations, eventtype, reason, messageFmt, args...)
}

// AnnotatedEventfCalls gets all the calls that were made to AnnotatedEventf.
// Check the length with:
//
//	len(mockedEventRecorder.AnnotatedEventfCalls())
func (mock *EventRecorderMock) AnnotatedEventfCalls() []struct {
	Object      runtime.Object
	Annotations map[string]string
	Eventtype   string
	Reason      string
	MessageFmt  string
	Args        []interface{}
} {
	var calls []struct {
		Object      runtime.Object
		Annotations map[string]string
		Eventtype   string
		Reason      string
		MessageFmt  string
		Args        []interface{}
	}
	mock.lockAnnotatedEventf.RLock()
	calls = mock.calls.AnnotatedEventf
	mock.lockAnnotatedEventf.RUnlock()
	return calls
}

// Event calls EventFunc.
func (mock *EventRecorderMock) Event(object runtime.Object, eventtype string, reason string, message string) {
	if mock.EventFunc == nil {
		panic("EventRecorderMock.EventFunc: method is nil but EventRecorder.Event was just called")
	}
	callInfo := struct {
		Object    runtime.Object
		Eventtype string
		Reason    string
		Message   string
	}{
		Object:    object,
		Eventtype: eventtype,
		Reason:    reason,
		Message:   message,
	}
	mock.lockEvent.Lock()
	mock.calls.Event = append(mock.calls.Event, callInfo)
	mock.lockEvent.Unlock()
	mock.EventFunc(object, eventtype, reason, message)
}

// EventCalls gets all the calls that were made to Event.
// Check the length with:
//
//	len(mockedEventRecorder.EventCalls())
func (mock *EventRecorderMock) EventCalls() []struct {
	Object    runtime.Object
	Eventtype string
	Reason    string
	Message   string
} {
	var calls []struct {
		Object    runtime.Object
		Eventtype string
		Reason    string
		Message   string
	}
	mock.lockEvent.RLock()
	calls = mock.calls.Event
	mock.lockEvent.RUnlock()
	return calls
}

// Eventf calls EventfFunc.
func (mock *EventRecorderMock) Eventf(object runtime.Object, eventtype string, reason string, messageFmt string, args ...interface{}) {
	if mock.EventfFunc == nil {
		panic("EventRecorderMock.EventfFunc: method is nil but EventRecorder.Eventf was just called")
	}
	callInfo := struct {
		Object     runtime.Object
		Eventtype  string
		Reason     string
		MessageFmt string
		Args       []interface{}
	}{
		Object:     object,
		Eventtype:  eventtype,
		Reason:     reason,
		MessageFmt: messageFmt,
		Args:       args,
	}
	mock.lockEventf.Lock()
	mock.calls.Eventf = append(mock.calls.Eventf, callInfo)
	mock.lockEventf.Unlock()
	mock.EventfFunc(object, eventtype, reason, messageFmt, args...)
}

// EventfCalls gets all the calls that were made to Eventf.
// Check the length with:
//
//	len(mockedEventRecorder.EventfCalls())
func (mock *EventRecorderMock) EventfCalls() []struct {
	Object     runtime.Object
	Eventtype  string
	Reason     string
	MessageFmt string
	Args       []interface{}
} {
	var calls []struct {
		Object     runtime.Object
		Eventtype  string
		Reason     string
		MessageFmt string
		Args       []interface{}
	}
	mock.lockEventf.RLock()
	calls = mock.calls.Eventf
	mock.lockEventf.RUnlock()
	return calls
}

// Ensure, that IBlockDeviceServiceMock does implement IBlockDeviceService.
// If this is not the case, regenerate this file with moq.
var _ IBlockDeviceService = &IBlockDeviceServiceMock{}

// IBlockDeviceServiceMock is a mock implementation of IBlockDeviceService.
//
//	func TestSomethingThatUsesIBlockDeviceService(t *testing.T) {
//
//		// make and configure a mocked IBlockDeviceService
//		mockedIBlockDeviceService := &IBlockDeviceServiceMock{
//			CountBlockDevicesAttachedToVmFunc: func(ctx context.Context, vm *virtv2.VirtualMachine) (int, error) {
//				panic("mock out the CountBlockDevicesAttachedToVm method")
//			},
//		}
//
//		// use mockedIBlockDeviceService in code that requires IBlockDeviceService
//		// and then make assertions.
//
//	}
type IBlockDeviceServiceMock struct {
	// CountBlockDevicesAttachedToVmFunc mocks the CountBlockDevicesAttachedToVm method.
	CountBlockDevicesAttachedToVmFunc func(ctx context.Context, vm *virtv2.VirtualMachine) (int, error)

	// calls tracks calls to the methods.
	calls struct {
		// CountBlockDevicesAttachedToVm holds details about calls to the CountBlockDevicesAttachedToVm method.
		CountBlockDevicesAttachedToVm []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// VM is the vm argument value.
			VM *virtv2.VirtualMachine
		}
	}
	lockCountBlockDevicesAttachedToVm sync.RWMutex
}

// CountBlockDevicesAttachedToVm calls CountBlockDevicesAttachedToVmFunc.
func (mock *IBlockDeviceServiceMock) CountBlockDevicesAttachedToVm(ctx context.Context, vm *virtv2.VirtualMachine) (int, error) {
	if mock.CountBlockDevicesAttachedToVmFunc == nil {
		panic("IBlockDeviceServiceMock.CountBlockDevicesAttachedToVmFunc: method is nil but IBlockDeviceService.CountBlockDevicesAttachedToVm was just called")
	}
	callInfo := struct {
		Ctx context.Context
		VM  *virtv2.VirtualMachine
	}{
		Ctx: ctx,
		VM:  vm,
	}
	mock.lockCountBlockDevicesAttachedToVm.Lock()
	mock.calls.CountBlockDevicesAttachedToVm = append(mock.calls.CountBlockDevicesAttachedToVm, callInfo)
	mock.lockCountBlockDevicesAttachedToVm.Unlock()
	return mock.CountBlockDevicesAttachedToVmFunc(ctx, vm)
}

// CountBlockDevicesAttachedToVmCalls gets all the calls that were made to CountBlockDevicesAttachedToVm.
// Check the length with:
//
//	len(mockedIBlockDeviceService.CountBlockDevicesAttachedToVmCalls())
func (mock *IBlockDeviceServiceMock) CountBlockDevicesAttachedToVmCalls() []struct {
	Ctx context.Context
	VM  *virtv2.VirtualMachine
} {
	var calls []struct {
		Ctx context.Context
		VM  *virtv2.VirtualMachine
	}
	mock.lockCountBlockDevicesAttachedToVm.RLock()
	calls = mock.calls.CountBlockDevicesAttachedToVm
	mock.lockCountBlockDevicesAttachedToVm.RUnlock()
	return calls
}
