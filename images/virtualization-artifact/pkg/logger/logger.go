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
	"slices"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/klog/v2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/deckhouse/deckhouse/pkg/log"
)

type Output string

const (
	Stdout  Output = "stdout"
	Stderr  Output = "stderr"
	Discard Output = "discard"
)

const DefaultLogLevel = log.LevelInfo

var DefaultLogOutput = os.Stdout

func NewLogger(level, output string, debugVerbosity int) *log.Logger {
	return log.NewLogger(log.Options{
		Level:  detectLogLevel(level, debugVerbosity),
		Output: detectLogOutput(output),
	})
}

type Factory func(controllerName string) *log.Logger

func NewFactory(level, output string, debugVerbosity int, controllerDebugList []string) Factory {
	slogLevel := detectLogLevel(level, debugVerbosity)

	return func(controllerName string) *log.Logger {
		if slices.Contains(controllerDebugList, controllerName) {
			if debugVerbosity != 0 {
				slogLevel = slog.Level(-1 * debugVerbosity)
			} else {
				slogLevel = log.LevelDebug.Level()
			}
		}

		return log.NewLogger(log.Options{
			Level:  slogLevel,
			Output: detectLogOutput(output),
		}).With(SlogController(controllerName))
	}
}

func detectLogLevel(level string, debugVerbosity int) slog.Level {
	switch strings.ToLower(level) {
	case "fatal":
		return log.LevelFatal.Level()
	case "error":
		return log.LevelError.Level()
	case "warn":
		return log.LevelWarn.Level()
	case "info":
		return log.LevelInfo.Level()
	case "debug":
		if debugVerbosity != 0 {
			return slog.Level(-1 * debugVerbosity)
		}

		return log.LevelDebug.Level()
	case "trace":
		return log.LevelTrace.Level()
	default:
		return DefaultLogLevel.Level()
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

func SetDefaultLogger(l *log.Logger) {
	slog.SetDefault(slog.New(l.Handler()))
	log.SetDefault(l)
	fromSlog := logr.FromSlogHandler(l.Handler())
	logf.SetLogger(fromSlog)
	klog.SetLogger(fromSlog)
}
