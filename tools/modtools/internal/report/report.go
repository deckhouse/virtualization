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

// Package report provides the shared result model used by all modtools checks:
// typed messages, a grouped report printer, and helpers that translate an exit
// code into the sentinel error cobra needs to fail with a non-zero status.
package report

import (
	"errors"
	"fmt"
	"strings"
)

// ErrFailed is returned by Result/ExitErr so cobra's Execute exits non-zero.
// The commands print their own report, so the root command silences errors and
// main only needs the non-nil signal.
var ErrFailed = errors.New("checks failed")

type MsgType string

const (
	TypeOK    MsgType = "OK"
	TypeSkip  MsgType = "Skip"
	TypeError MsgType = "ERROR"
)

type Message struct {
	Type     MsgType
	FileName string
	Message  string
	Details  string
}

func NewOK(fileName string) Message {
	return Message{Type: TypeOK, FileName: fileName}
}

func NewSkip(fileName, msg string) Message {
	return Message{Type: TypeSkip, FileName: fileName, Message: msg}
}

func NewError(fileName, msg, details string) Message {
	return Message{Type: TypeError, FileName: fileName, Message: msg, Details: details}
}

func (msg Message) Format() string {
	res := ""
	if msg.Message == "" {
		res += fmt.Sprintf("  * %s ... %s", msg.FileName, msg.Type)
	} else {
		res += fmt.Sprintf("  * %s ... %s: %s", msg.FileName, msg.Type, msg.Message)
	}
	if msg.Details != "" {
		res += "\n" + indentTextBlock(msg.Details, 6)
	}
	return res
}

type Messages struct {
	messages []Message
}

func NewMessages() *Messages {
	return &Messages{messages: make([]Message, 0)}
}

func (m *Messages) Add(msg Message) {
	m.messages = append(m.messages, msg)
}

func (m *Messages) count(t MsgType) int {
	res := 0
	for _, msg := range m.messages {
		if msg.Type == t {
			res++
		}
	}
	return res
}

func (m *Messages) CountErrors() int { return m.count(TypeError) }

func (m *Messages) PrintReport() {
	sections := []struct {
		title string
		typ   MsgType
	}{
		{"Skipped:", TypeSkip},
		{"OK:", TypeOK},
		{"ERRORS:", TypeError},
	}
	for _, s := range sections {
		if m.count(s.typ) == 0 {
			continue
		}
		fmt.Println(s.title)
		for _, msg := range m.messages {
			if msg.Type == s.typ {
				fmt.Println(msg.Format())
			}
		}
	}
}

// Result prints a human-readable verdict and returns ErrFailed on a non-zero code.
func Result(exitCode int) error {
	if exitCode == 0 {
		fmt.Println("Validation successful.")
		return nil
	}
	fmt.Println("Validation failed.")
	return ErrFailed
}

// ExitErr maps a non-zero exit code to ErrFailed without printing a verdict
// (the caller already produced its own report).
func ExitErr(exitCode int) error {
	if exitCode == 0 {
		return nil
	}
	return ErrFailed
}

func indentTextBlock(msg string, n int) string {
	lines := strings.Split(msg, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.Repeat(" ", n))
		b.WriteString(line)
	}
	return b.String()
}
