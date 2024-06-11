package main

import "fmt"

const OKType = "OK"
const SkipType = "Skip"
const ErrorType = "ERROR"

type Message struct {
	Type     string
	FileName string
	Message  string
	Details  string
}

func (msg Message) IsError() bool {
	return msg.Type == ErrorType
}

func (msg Message) IsSkip() bool {
	return msg.Type == SkipType
}

func (msg Message) IsOK() bool {
	return msg.Type == OKType
}

func (msg Message) Format() string {
	res := ""
	if msg.Message == "" {
		res += fmt.Sprintf("  * %s ... %s", msg.FileName, msg.Type)
	} else {
		res += fmt.Sprintf("  * %s ... %s: %s", msg.FileName, msg.Type, msg.Message)
	}
	if msg.Details != "" {
		res += fmt.Sprintf("  * %s ... %s: %s", msg.FileName, msg.Type, msg.Message)
		//res += "\n" + indentTextBlock(msg.Details, 6)
	}
	return res
}

func NewOK(fileName string) Message {
	return Message{
		Type:     OKType,
		FileName: fileName,
	}
}

func NewSkip(fileName string, msg string) Message {
	return Message{
		Type:     SkipType,
		FileName: fileName,
		Message:  msg,
	}
}

func NewError(fileName string, msg string, details string) Message {
	return Message{
		Type:     ErrorType,
		FileName: fileName,
		Message:  msg,
		Details:  details,
	}
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

func (m *Messages) PrintReport() {
	if m.CountSkip() > 0 {
		fmt.Println("Skipped:")
		for _, msg := range m.messages {
			if msg.IsSkip() {
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
