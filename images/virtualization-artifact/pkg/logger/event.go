package logger

import (
	"log/slog"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
)

const (
	eventReasonLabel       = "reason"
	eventTypeLabel         = "type"
	eventInvolvedName      = "involvedName"
	eventInvolvedNamespace = "involvedNamespace"
	eventInvolvedKind      = "involvedKind"
)

// infoLogger is local interface to use Info method from different loggers.
type infoLogger interface {
	Info(msg string, args ...any)
}

func anyLoggerWith(logger infoLogger, args ...any) infoLogger {
	switch l := logger.(type) {
	case *log.Logger:
		return l.With(args) //args any
	case *slog.Logger:
		return l.With(args) // args any
	case logr.Logger:
		l.WithValues()
	}
	return logger
}

func EventInfo(logger infoLogger, ev v1.Event) {
	logger.Info(ev.Message,
		eventReasonLabel, ev.Reason,
		eventTypeLabel, ev.Type,
		eventInvolvedName, ev.InvolvedObject.Name,
		eventInvolvedNamespace, ev.InvolvedObject.Namespace,
		eventInvolvedKind, ev.InvolvedObject.Kind,
	)
}
