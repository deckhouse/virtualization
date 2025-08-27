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

package errlogger

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

type warning string

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

type LogStream struct {
	Cancel             context.CancelFunc
	ContainerStartedAt v1.Time
	LogStreamCmd       *exec.Cmd
	LogStreamWaitGroup *sync.WaitGroup
	PodName            string
	Stderr             io.ReadCloser
	Stdout             io.ReadCloser
}

func (l *LogStream) ConnectStderr() {
	GinkgoHelper()
	stderr, err := l.LogStreamCmd.StderrPipe()
	Expect(err).NotTo(HaveOccurred(), "failed to obtain the `Virtualization-controller` STDERR stream: %s", l.PodName)
	l.Stderr = stderr
}

func (l *LogStream) ConnectStdout() {
	GinkgoHelper()
	stdout, err := l.LogStreamCmd.StdoutPipe()
	Expect(err).NotTo(HaveOccurred(), "failed to obtain the `Virtualization-controller` STDOUT stream: %s", l.PodName)
	l.Stdout = stdout
}

func (l *LogStream) ParseStderr() {
	GinkgoHelper()
	defer GinkgoRecover()
	defer l.LogStreamWaitGroup.Done()

	scanner := bufio.NewScanner(l.Stderr)
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		_, writeErr := GinkgoWriter.Write([]byte(fmt.Sprintf("%s%s%s\n", Red, scanner.Text(), Reset)))
		Expect(writeErr).NotTo(HaveOccurred())
	}
	parseScanError(scanner.Err(), "STDERR")
}

func (l *LogStream) ParseStdout(excludedPatterns []string, excludedRegexpPattens []regexp.Regexp, startTime time.Time) {
	GinkgoHelper()
	defer GinkgoRecover()
	defer l.LogStreamWaitGroup.Done()

	errFlag := false
	scanner := bufio.NewScanner(l.Stdout)
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		var entry LogEntry
		rawEntry := strings.TrimPrefix(scanner.Text(), "0")
		err := json.Unmarshal([]byte(rawEntry), &entry)
		Expect(err).NotTo(HaveOccurred(), "error parsing JSON")
		if entry.Level == LevelError && !isMsgIgnoredByPattern(rawEntry, excludedPatterns, excludedRegexpPattens) {
			errTime, err := time.Parse(time.RFC3339, entry.Time)
			Expect(err).NotTo(HaveOccurred(), "failed to parse error timestamp")
			if errTime.After(startTime) {
				errFlag = true
				jsonData, err := json.MarshalIndent(entry, "", "  ")
				Expect(err).NotTo(HaveOccurred(), "error converting to JSON")
				msg := formatMessage(
					"this is the `Virtualization-controller` error! not the current `Ginkgo` context error:",
					string(jsonData),
					Red,
				)
				_, writeErr := GinkgoWriter.Write([]byte(msg))
				Expect(writeErr).NotTo(HaveOccurred())
			}
		}
	}
	parseScanError(scanner.Err(), "STDOUT")
	Expect(errFlag).ShouldNot(BeTrue(), "errors have appeared in the `Virtualization-controller` logs")
}

func (l *LogStream) WaitCmd() (warning, error) {
	err := l.LogStreamCmd.Wait()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				msg := formatMessage(
					"Warning!",
					fmt.Sprintf("The process was terminated with the %q signal.", status.Signal()),
					Yellow,
				)
				return warning(msg), nil
			}
		}
		return "", fmt.Errorf("the command %q has been finished with the error: %w", l.LogStreamCmd.String(), err)
	}
	return "", nil
}

func (l *LogStream) Start() {
	GinkgoHelper()
	err := l.LogStreamCmd.Start()
	Expect(err).NotTo(HaveOccurred(), "failed to start the `Virtualization-controller` log stream: %s", l.PodName)
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

func isMsgIgnoredByPattern(msg string, patterns []string, regexpPatterns []regexp.Regexp) bool {
	for _, s := range patterns {
		if strings.Contains(msg, s) {
			return true
		}
	}
	for _, r := range regexpPatterns {
		if r.MatchString(msg) {
			return true
		}
	}
	return false
}

// stream: "STDERR" | "STDOUT"
func parseScanError(err error, stream string) {
	var pathError *fs.PathError
	if errors.As(err, &pathError) {
		msg := formatMessage(
			fmt.Sprintf("Warning! The %q file already closed.", stream),
			"This may be caused by canceling the log stream process.",
			Yellow,
		)
		_, writeErr := GinkgoWriter.Write([]byte(msg))
		Expect(writeErr).NotTo(HaveOccurred())
	} else {
		Expect(err).NotTo(HaveOccurred(), "failed to scan the %q stream)", stream)
	}
}
