package logger

type Option interface{}

type DebugOption struct{}

func NewDebugOption() *DebugOption {
	return &DebugOption{}
}
