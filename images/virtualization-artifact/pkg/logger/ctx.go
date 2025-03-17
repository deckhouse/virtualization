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

package logger

import (
	"context"
	"log/slog"

	"github.com/go-logr/logr"
)

// ToContext adds logger to context.
func ToContext(ctx context.Context, l *slog.Logger) context.Context {
	return logr.NewContextWithSlogLogger(ctx, l)
}

// FromContext returns logger from context.
func FromContext(ctx context.Context) *slog.Logger {
	if l := logr.FromContextAsSlogLogger(ctx); l != nil {
		return l
	}
	missingLogger := slog.Default().With(slog.String("logger", "missing_from_context"))
	missingLogger.Warn("Logger was not found in context, using default")
	return missingLogger
}

func GetDataSourceContext(ctx context.Context, ds string) (*slog.Logger, context.Context) {
	log := FromContext(ctx).With(SlogHandler(ds))
	return log, ToContext(context.WithoutCancel(ctx), log)
}

func GetHandlerContext(ctx context.Context, handler string) (*slog.Logger, context.Context) {
	log := FromContext(ctx).With(SlogHandler(handler))
	return log, ToContext(context.WithoutCancel(ctx), log)
}
