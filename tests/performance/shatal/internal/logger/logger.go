package logger

import (
	"log/slog"
	"os"
	"strings"
)

func New(options ...Option) *slog.Logger {
	var handlerOptions slog.HandlerOptions

	for _, option := range options {
		switch option.(type) {
		case *DebugOption:
			handlerOptions.Level = slog.LevelDebug
		default:
		}
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &handlerOptions))
}

type Writer struct {
	log *slog.Logger
}

func NewWriter(log *slog.Logger) *Writer {
	return &Writer{
		log: log,
	}
}

func (w *Writer) Write(p []byte) (n int, err error) {
	record := string(p)

	if strings.Contains(record, "Cannot evict pod as it would violate the pod's disruption budget") || strings.HasPrefix(record, "evicting pod") {
		return len(p), nil
	}

	w.log.Debug(string(p))
	return len(p), nil
}
