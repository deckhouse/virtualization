/*
Copyright 2026 Flant JSC

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

package udev

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
)

// Action represents the action type from kobject events
type Action string

const (
	ActionAdd     Action = "add"
	ActionRemove  Action = "remove"
	ActionChange  Action = "change"
	ActionMove    Action = "move"
	ActionOnline  Action = "online"
	ActionOffline Action = "offline"
	ActionBind    Action = "bind"
	ActionUnbind  Action = "unbind"
)

// String returns string representation of Action
func (a Action) String() string {
	return string(a)
}

// ParseAction parses a string into Action
func ParseAction(s string) (Action, error) {
	a := Action(s)
	switch a {
	case ActionAdd, ActionRemove, ActionChange, ActionMove, ActionOnline, ActionOffline, ActionBind, ActionUnbind:
		return a, nil
	default:
		return "", fmt.Errorf("unknown action: %s", s)
	}
}

// UEvent represents a parsed uevent from netlink
type UEvent struct {
	// Action is the event action (add, remove, change, etc.)
	Action Action
	// KObj is the kernel object path (e.g., /devices/pci0000:00/.../3-2)
	KObj string
	// Env contains the environment variables from the event
	Env map[string]string
}

// Subsystem returns the subsystem from the event environment
func (e *UEvent) Subsystem() string {
	return e.Env["SUBSYSTEM"]
}

// DevType returns the device type from the event environment
func (e *UEvent) DevType() string {
	return e.Env["DEVTYPE"]
}

// DevPath returns the device path from the event environment
func (e *UEvent) DevPath() string {
	return e.Env["DEVPATH"]
}

// ErrInvalidUEvent is returned when an uevent cannot be parsed
var ErrInvalidUEvent = errors.New("invalid uevent format")

// ParseUEvent parses a raw uevent message from netlink
func ParseUEvent(raw []byte) (*UEvent, error) {
	// Split by null bytes
	fields := bytes.Split(raw, []byte{0x00})
	if len(fields) < 2 {
		return nil, ErrInvalidUEvent
	}

	// First field is like "add@/devices/pci0000:00/..."
	headers := bytes.Split(fields[0], []byte("@"))
	if len(headers) != 2 {
		return nil, ErrInvalidUEvent
	}

	action, err := ParseAction(string(headers[0]))
	if err != nil {
		return nil, err
	}

	e := &UEvent{
		Action: action,
		KObj:   string(headers[1]),
		Env:    make(map[string]string),
	}

	// Parse environment variables
	for _, envs := range fields[1 : len(fields)-1] {
		if len(envs) == 0 {
			continue
		}
		key, value, found := strings.Cut(string(envs), "=")
		if found {
			e.Env[key] = value
		}
	}

	return e, nil
}
