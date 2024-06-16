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

package main

import "fmt"

const (
	TypeAdd   = "Add"
	TypeSkip  = "Skip"
	TypeError = "ERROR"
)

type Message struct {
	Type     string
	FileName string
	Message  string
	Details  string
}

func NewAdd(fileName string) Message {
	return Message{
		Type:     TypeAdd,
		FileName: fileName,
	}
}

func NewSkip(fileName, msg string) Message {
	return Message{
		Type:     TypeSkip,
		FileName: fileName,
		Message:  msg,
	}
}

func NewError(fileName, msg, details string) Message {
	return Message{
		Type:     TypeError,
		FileName: fileName,
		Message:  msg,
		Details:  details,
	}
}

func (msg Message) IsError() bool {
	return msg.Type == TypeError
}

func (msg Message) IsSkip() bool {
	return msg.Type == TypeSkip
}

func (msg Message) IsAdd() bool {
	return msg.Type == TypeAdd
}

type Messages struct {
	messages []Message
}

func NewMessages() *Messages {
	return &Messages{
		messages: make([]Message, 0),
	}
}

func (m *Messages) Add(msg Message) {
	m.messages = append(m.messages, msg)
}

func (m *Messages) CountAdd() int {
	res := 0
	for _, msg := range m.messages {
		if msg.IsAdd() {
			res++
		}
	}
	return res
}

func (m *Messages) CountSkip() int {
	res := 0
	for _, msg := range m.messages {
		if msg.IsSkip() {
			res++
		}
	}
	return res
}

func (m *Messages) CountErrors() int {
	res := 0
	for _, msg := range m.messages {
		if msg.IsError() {
			res++
		}
	}
	return res
}

func (msg Message) Format() string {
	res := ""
	if msg.Message == "" {
		res += fmt.Sprintf("  * %s ... %s", msg.FileName, msg.Type)
	} else {
		res += fmt.Sprintf("  * %s ... %s: %s", msg.FileName, msg.Type, msg.Message)
	}
	if msg.Details != "" {
		res += "\n" + msg.Details
	}
	return res
}

func (m *Messages) PrintReport() {
	if m.CountSkip() > 0 {
		fmt.Println("Skipped:")
		for _, msg := range m.messages {
			if msg.IsSkip() {
				fmt.Println(msg.Format())
			}
		}
	}
	if m.CountAdd() > 0 {
		fmt.Println("Add:")
		for _, msg := range m.messages {
			if msg.IsAdd() {
				fmt.Println(msg.Format())
			}
		}
	}
	if m.CountErrors() > 0 {
		fmt.Println("ERRORS:")
		for _, msg := range m.messages {
			if msg.IsError() {
				fmt.Println(msg.Format())
			}
		}
	}
}
