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

package controller

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"time"
)

const (
	Red         = "\033[31m"
	Yellow      = "\033[33m"
	Green       = "\033[32m"
	Reset       = "\033[0m"
	Bold        = "\033[1m"
	LevelError  = "error"
	maxCapacity = 1024 << 10
)

type LogEntry struct {
	Level       string `json:"level"`
	Message     string `json:"msg"`
	Err         string `json:"err"`
	Controller  string `json:"controller"`
	Handler     string `json:"handler"`
	DataSource  string `json:"ds"`
	Collector   string `json:"collector"`
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	ReconcileID string `json:"reconcileID"`
	Time        string `json:"time"`
}

type ErrStreamer struct {
	excludedPatterns      [][]byte
	excludedRegexpPattens []regexp.Regexp
}

func NewErrStreamer(excludedPatterns []string, excludedRegexpPattens []regexp.Regexp) *ErrStreamer {
	patterns := make([][]byte, len(excludedPatterns))
	for i, s := range excludedPatterns {
		patterns[i] = []byte(s)
	}
	return &ErrStreamer{
		excludedPatterns:      patterns,
		excludedRegexpPattens: excludedRegexpPattens,
	}
}

func (l *ErrStreamer) Stream(r io.Reader, w io.Writer) (int, error) {
	startTime := time.Now()

	scanner := bufio.NewScanner(r)
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	num := 0

	for scanner.Scan() {
		rawEntry := scanner.Bytes()

		var entry LogEntry
		err := json.Unmarshal(rawEntry, &entry)
		if err != nil {
			continue
		}

		if entry.Level == LevelError && !l.isMsgIgnoredByPattern(rawEntry) {
			errTime, err := time.Parse(time.RFC3339, entry.Time)
			if err != nil {
				continue
			}
			if errTime.After(startTime) {
				jsonData, err := json.MarshalIndent(entry, "", "  ")
				if err != nil {
					continue
				}
				msg := formatMessage(
					"this is the `Virtualization-controller` error! not the current `Ginkgo` context error:",
					string(jsonData),
					Red,
				)
				n, _ := w.Write([]byte(msg))
				num += n
			}
		}
	}

	return num, scanner.Err()
}

func (l *ErrStreamer) isMsgIgnoredByPattern(msg []byte) bool {
	for _, s := range l.excludedPatterns {
		if bytes.Contains(msg, s) {
			return true
		}
	}
	for _, r := range l.excludedRegexpPattens {
		if r.Match(msg) {
			return true
		}
	}
	return false
}

func formatMessage(header, msg, color string) string {
	return fmt.Sprintf(
		"%s%s\n%s\n%s%s%s%s\n",
		color,
		Bold,
		header,
		Reset,
		color,
		msg,
		Reset,
	)
}
