/*
Copyright 2026 Flant JSC

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

package cli

import (
	"log/slog"
	"os"
	"strconv"
)

func GetEnv[T any](key string, parse func(string) (T, error), defaultValue T) T {
	value, ok := os.LookupEnv(key)
	if !ok {
		return defaultValue
	}

	parsed, err := parse(value)
	if err != nil {
		slog.Warn("failed to parse env value", slog.String("key", key), slog.String("value", value), slog.String("error", err.Error()))
		return defaultValue
	}

	return parsed
}

func GetStringEnv(key, defaultValue string) string {
	return GetEnv(key, func(s string) (string, error) { return s, nil }, defaultValue)
}

func GetBoolEnv(key string, defaultValue bool) bool {
	return GetEnv(key, strconv.ParseBool, defaultValue)
}

func GetIntEnv(key string, defaultValue int) int {
	return GetEnv(key, strconv.Atoi, defaultValue)
}
