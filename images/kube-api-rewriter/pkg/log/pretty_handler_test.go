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
