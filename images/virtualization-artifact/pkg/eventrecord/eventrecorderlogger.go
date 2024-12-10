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

package eventrecord

import (
	"fmt"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	eventTypeLabel         = "eventType"
	reasonLabel            = "reason"
	involvedNameLabel      = "involvedName"
	involvedNamespaceLabel = "involvedNamespace"
	involvedKindLabel      = "involvedKind"
	relatedNameLabel       = "relatedName"
	relatedNamespaceLabel  = "relatedNamespace"
	relatedKindLabel       = "relatedKind"
)

// infoLogger is local interface to use Info method from different loggers.
type infoLogger interface {
	Info(msg string, args ...any)
}

// EventRecorderLogger is a wrapper around client-go's EventRecorder to record Events with logging.
type EventRecorderLogger interface {
	Event(object client.Object, eventtype, reason, message string)

	// Eventf is just like Event, but with Sprintf for the message field.
	Eventf(involved client.Object, eventtype, reason, messageFmt string, args ...interface{})

	// AnnotatedEventf is just like eventf, but with annotations attached
	AnnotatedEventf(involved client.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{})

	WithLogging(logger infoLogger) EventRecorderLogger
}

func NewEventRecorderLogger(recorder record.EventRecorder) EventRecorderLogger {
	return &EventRecorderLoggerImpl{recorder: recorder}
}

// EventRecorderLoggerImpl implements Event recorder that also log event object.
type EventRecorderLoggerImpl struct {
	recorder record.EventRecorder
	logger   infoLogger
}

func (e *EventRecorderLoggerImpl) WithLogging(logger infoLogger) EventRecorderLogger {
	return &EventRecorderLoggerImpl{
		recorder: e.recorder,
		logger:   logger,
	}
}

//// FromObject records an event from an existing event object.
//// Notice that only involvedObject, type, reason, and message are used.
//func (e *EventRecorderLoggerImpl) FromObject(ev *Event) {
//	e.recorder.Event(ev.InvolvedObject, ev.Type, ev.Reason, ev.Message)
//}

//// EventRelated records an event and adds related object info to log.
//func (e *EventRecorderLoggerImpl) EventRelated(object client.Object, related client.Object, eventtype, reason, message string) {
//	e.recorder.Event(object, eventtype, reason, message)
//	e.log(&Event{
//		InvolvedObject: object,
//		RelatedObject:  related,
//		Type:           eventtype,
//		Reason:         reason,
//		Message:        message,
//	})
//}

// Event calls EventRecorder.Event as-is.
func (e *EventRecorderLoggerImpl) Event(object client.Object, eventtype, reason, message string) {
	e.logf(object, eventtype, reason, message)
	e.recorder.Event(object, eventtype, reason, message)
}

// Eventf calls EventRecorder.Eventf as-is.
func (e *EventRecorderLoggerImpl) Eventf(object client.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	e.logf(object, eventtype, reason, messageFmt, args...)
	e.recorder.Eventf(object, eventtype, reason, messageFmt, args)
}

// AnnotatedEventf calls EventRecorder.AnnotatedEventf as-is.
func (e *EventRecorderLoggerImpl) AnnotatedEventf(object client.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	e.logf(object, eventtype, reason, messageFmt, args...)
	e.recorder.AnnotatedEventf(object, annotations, eventtype, reason, messageFmt, args)
}

func (e *EventRecorderLoggerImpl) log(ev *Event) {
	if e.logger == nil {
		return
	}
	args := []any{
		eventTypeLabel, ev.Type,
		reasonLabel, ev.Reason,
		involvedNameLabel, ev.InvolvedObject.GetName(),
		involvedNamespaceLabel, ev.InvolvedObject.GetNamespace(),
		involvedKindLabel, ev.InvolvedObject.GetObjectKind().GroupVersionKind().Kind,
	}
	if ev.RelatedObject != nil {
		args = append(args,
			relatedNameLabel, ev.RelatedObject.GetName(),
			relatedNamespaceLabel, ev.RelatedObject.GetNamespace(),
			relatedKindLabel, ev.RelatedObject.GetObjectKind().GroupVersionKind().Kind,
		)
	}
	e.logger.Info(ev.Message, args...)
}

func (e *EventRecorderLoggerImpl) logf(involved client.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	if e.logger == nil {
		return
	}
	e.logger.Info(
		fmt.Sprintf(messageFmt, args...),
		eventTypeLabel, eventtype,
		reasonLabel, reason,
		involvedNameLabel, involved.GetName(),
		involvedNamespaceLabel, involved.GetNamespace(),
		involvedKindLabel, involved.GetObjectKind().GroupVersionKind().Kind,
	)
}
