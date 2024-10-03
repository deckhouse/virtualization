package proxy

import (
	"context"
	"log/slog"

	"kube-api-proxy/pkg/labels"
)

func LoggerWithCommonAttrs(ctx context.Context, attrs ...any) *slog.Logger {
	logger := slog.Default()
	logger = logger.With(
		slog.String("proxy.name", labels.NameFromContext(ctx)),
		slog.String("resource", labels.ResourceFromContext(ctx)),
		slog.String("method", labels.MethodFromContext(ctx)),
	)
	return logger.With(attrs)
}
