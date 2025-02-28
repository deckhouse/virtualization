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
	"strings"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	eventTypeLabel         = "eventType"
	reasonLabel            = "reason"
	involvedNameLabel      = "involvedName"
	involvedNamespaceLabel = "involvedNamespace"
	involvedKindLabel      = "involvedKind"
)

//go:generate moq -rm -out mock.go . EventRecorderLogger

// InfoLogger is local interface to use Info method from different loggers.
type InfoLogger interface {
	Info(msg string, args ...any)
}

type recorderProducer interface {
	GetEventRecorderFor(name string) record.EventRecorder
}

// EventRecorderLogger is a wrapper around client-go's EventRecorder to record Events with logging.
type EventRecorderLogger interface {
	Event(object client.Object, eventtype, reason, message string)

	// Eventf is just like Event, but with Sprintf for the message field.
	Eventf(involved client.Object, eventtype, reason, messageFmt string, args ...interface{})

	// AnnotatedEventf is just like eventf, but with annotations attached
	AnnotatedEventf(involved client.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{})

	WithLogging(logger InfoLogger) EventRecorderLogger
}

// NewEventRecorderLogger implements EventRecorderLogger.
// It creates recorder for each reason to workaround low qps limit of 1 event per 5 minutes.
func NewEventRecorderLogger(recorderProducer recorderProducer, controllerName string) EventRecorderLogger {
	return &EventRecorderLoggerImpl{
		recorderProducer: recorderProducer,
		controllerName:   controllerName,
	}
}

// EventRecorderLoggerImpl implements Event recorder that also log event object.
type EventRecorderLoggerImpl struct {
	controllerName   string
	recorderProducer recorderProducer
	logger           InfoLogger
}

func (e *EventRecorderLoggerImpl) WithLogging(logger InfoLogger) EventRecorderLogger {
	return &EventRecorderLoggerImpl{
		controllerName:   e.controllerName,
		recorderProducer: e.recorderProducer,
		logger:           logger,
	}
}

// Event logs info about event and records it as Event resource.
func (e *EventRecorderLoggerImpl) Event(object client.Object, eventtype, reason, message string) {
	e.logf(object, eventtype, reason, message, nil)
	recorder := e.recorderProducer.GetEventRecorderFor(e.recorderKey(reason))
	recorder.Event(object, eventtype, reason, message)
}

// Eventf logs info about event and records it as Event resource. The difference with Event method is that it formats message.
func (e *EventRecorderLoggerImpl) Eventf(object client.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	e.logf(object, eventtype, reason, messageFmt, args...)
	recorder := e.recorderProducer.GetEventRecorderFor(e.recorderKey(reason))
	recorder.Eventf(object, eventtype, reason, messageFmt, args...)
}

// AnnotatedEventf logs info about event and records it as Event resource.
// The difference with the Event method is that it formats message and add annotations to the Event resource.
func (e *EventRecorderLoggerImpl) AnnotatedEventf(object client.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	e.logf(object, eventtype, reason, messageFmt, args...)
	recorder := e.recorderProducer.GetEventRecorderFor(e.recorderKey(reason))
	recorder.AnnotatedEventf(object, annotations, eventtype, reason, messageFmt, args...)
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

func (e *EventRecorderLoggerImpl) recorderKey(reason string) string {
	return strings.Join([]string{
		e.controllerName,
		"/",
		reason,
	}, "")
}
