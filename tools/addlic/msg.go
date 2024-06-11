package main

import "fmt"

const TypeOK = "OK"
const TypeAdd = "Add"
const TypeSkip = "Skip"
const TypeError = "ERROR"

type Message struct {
	Type     string
	FileName string
	Message  string
	Details  string
}

func NewOK(fileName string) Message {
	return Message{
		Type:     TypeOK,
		FileName: fileName,
	}
}

func NewAdd(fileName string) Message {
	return Message{
		Type:     TypeAdd,
		FileName: fileName,
	}
}

func NewSkip(fileName string, msg string) Message {
	return Message{
		Type:     TypeSkip,
		FileName: fileName,
		Message:  msg,
	}
}

func NewError(fileName string, msg string, details string) Message {
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

func (msg Message) IsOK() bool {
	return msg.Type == TypeOK
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

func (m *Messages) CountOK() int {
	res := 0
	for _, msg := range m.messages {
		if msg.IsOK() {
			res++
		}
	}
	return res
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
	if m.CountOK() > 0 {
		fmt.Println("OK:")
		for _, msg := range m.messages {
			if msg.IsOK() {
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
