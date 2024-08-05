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
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/klog/v2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func New(opts Options) *slog.Logger {
	return slog.New(NewHandler(opts))
}

func SetDefaultLogger(l *slog.Logger) {
	slog.SetDefault(l)
	fromSlog := logr.FromSlogHandler(l.Handler())
	logf.SetLogger(fromSlog)
	klog.SetLogger(fromSlog)
}

type Format string

const (
	TextLog Format = "text"
	JSONLog Format = "json"
)

type Output string

const (
	Stdout  Output = "stdout"
	Stderr  Output = "stderr"
	Discard Output = "discard"
)

const (
	DefaultLogLevel  = slog.LevelInfo
	DefaultLogFormat = JSONLog
)

var DefaultLogOutput = os.Stdout

type Options struct {
	Level          string
	DebugVerbosity int
	Format         string
	Output         string
}

func NewHandler(opts Options) slog.Handler {
	logLevel := detectLogLevel(opts.Level, opts.DebugVerbosity)
	logFormat := detectLogFormat(opts.Format)
	logOutput := detectLogOutput(opts.Output)

	logHandlerOpts := &slog.HandlerOptions{Level: logLevel}
	switch logFormat {
	case TextLog:
		return slog.NewTextHandler(logOutput, logHandlerOpts)
	case JSONLog:
		return slog.NewJSONHandler(logOutput, logHandlerOpts)
	default:
		return slog.NewTextHandler(logOutput, logHandlerOpts)
	}
}

func detectLogLevel(level string, debugVerbosity int) slog.Level {
	switch strings.ToLower(level) {
	case "error":
		return slog.LevelError
	case "warn":
		return slog.LevelWarn
	case "info":
		return slog.LevelInfo
	case "debug":
		if debugVerbosity != 0 {
			return slog.Level(-1 * debugVerbosity)
		}

		return slog.LevelDebug
	default:
		return DefaultLogLevel
	}
}

func detectLogFormat(format string) Format {
	switch strings.ToLower(format) {
	case string(TextLog):
		return TextLog
	case string(JSONLog):
		return JSONLog
	default:
		return DefaultLogFormat
	}
}

func detectLogOutput(output string) io.Writer {
	switch strings.ToLower(output) {
	case string(Stdout):
		return os.Stdout
	case string(Stderr):
		return os.Stderr
	case string(Discard):
		return io.Discard
	default:
		return DefaultLogOutput
	}
}
