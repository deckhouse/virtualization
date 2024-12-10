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
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Event struct {
	// The object that this event is about.
	InvolvedObject client.Object

	// The top object that this event is related to. E.g. VirtualMachine for VirtualMachineOperation.
	// Only for logging, will not be reflected in the Event resource.
	RelatedObject client.Object

	// Type of this event (Normal, Warning), new types could be added in the future
	Type string

	// This should be a short, machine understandable string that gives the reason
	// for the transition into the object's current status.
	Reason string

	// A human-readable description of the status of this operation.
	Message string
}

func NewNormalEvent(involvedObject client.Object, reason, message string) *Event {
	return &Event{
		InvolvedObject: involvedObject,
		Type:           corev1.EventTypeNormal,
		Reason:         reason,
		Message:        message,
	}
}

func NewWarningEvent(involvedObject client.Object, reason, message string) *Event {
	return &Event{
		InvolvedObject: involvedObject,
		Type:           corev1.EventTypeWarning,
		Reason:         reason,
		Message:        message,
	}
}

func NewNormalEventWithRelated(involvedObject client.Object, relatedObject client.Object, reason, message string) *Event {
	ev := NewNormalEvent(involvedObject, reason, message)
	ev.RelatedObject = relatedObject
	return ev
}
