package log

import (
	"log/slog"
	"os"
	"testing"
)

func TestDefaultCustomHandler(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		//Level:       nil,
		//ReplaceAttr: nil,
	})))

	logg := slog.With(
		slog.Group("properties",
			slog.Int("width", 4000),
			slog.Int("height", 3000),
			slog.String("format", "jpeg"),
			slog.Group("nestedprops",
				slog.String("arg", "val"),
			),
		),
		slog.String("azaz", "foo"),
	)
	logg.Info("message with group",
		slog.Group("properties",
			slog.Int("width", 6000),
		),
	)

	// set PrettyHandler as default
	//dbgHandler := NewPrettyHandler(os.Stdout, nil)
	dbgHandler := NewPrettyHandler(os.Stdout, &slog.HandlerOptions{AddSource: true})

	slog.SetDefault(slog.New(dbgHandler))

	logger := slog.With(
		slog.String("arg1", "val1"),
		slog.String("body.diff", "+-+-+-+\n++--++--\n  + qwe\n  - azaz"),
		slog.Group("properties",
			slog.Int("width", 6000),
		),
	)

	logger.Info("info message")

	logger = slog.With(
		slog.String("arg1", "val1"),
		slog.String("body.diff", "+-+-+-+"),
	)
	logger.WithGroup("properties").Info("info message",
		slog.Int("width", 6000),
	)
}
