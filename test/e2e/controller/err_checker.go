/*
Copyright 2025 Flant JSC

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
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/onsi/ginkgo/v2"
	"golang.org/x/net/http2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

// isGoAwayError reports whether err is or contains http2.GoAwayError (e.g. when wrapped in errors.Join).
// GOAWAY is sent by the server when the connection is closed (e.g. after context cancel) and should not fail the test.
func isGoAwayError(err error) bool {
	var goAway *http2.GoAwayError
	if errors.As(err, &goAway) {
		return true
	}
	type multiUnwrap interface{ Unwrap() []error }
	if u, ok := err.(multiUnwrap); ok {
		for _, e := range u.Unwrap() {
			if isGoAwayError(e) {
				return true
			}
		}
	}
	return false
}

// isExpectedStreamShutdownError reports whether err is expected when we stop the log stream
// (e.g. we closed the stream, or the server sent GOAWAY after context cancel).
func isExpectedStreamShutdownError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return true
	}
	if isGoAwayError(err) {
		return true
	}
	// Client closed the stream: Read() returns "read on closed body" or "use of closed network connection".
	s := err.Error()
	if strings.Contains(s, "read on closed body") || strings.Contains(s, "use of closed network connection") {
		return true
	}
	return false
}

// LogChecker detects `v12n-controller` errors while the test suite is running.
type LogChecker struct {
	ctx     context.Context
	cancel  context.CancelFunc
	closers []io.Closer
	wg      *sync.WaitGroup

	resultNum int
	resultErr error
	mu        sync.Mutex
}

func (l *LogChecker) Start() error {
	l.ctx, l.cancel = context.WithCancel(context.Background())
	l.wg = &sync.WaitGroup{}

	kubeClient := framework.GetClients().KubeClient()
	pods, err := kubeClient.CoreV1().Pods(VirtualizationNamespace).List(l.ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{"app": VirtualizationController}).String(),
	})
	if err != nil {
		return fmt.Errorf("failed to obtain the `Virtualization-controller` pods: %w", err)
	}

	for _, p := range pods.Items {
		req := kubeClient.CoreV1().Pods(VirtualizationNamespace).GetLogs(p.Name, &corev1.PodLogOptions{
			Container: VirtualizationController,
			Follow:    true,
		})
		readCloser, err := req.Stream(l.ctx)
		if err != nil {
			return fmt.Errorf("failed to stream the `Virtualization-controller` logs: %w", err)
		}

		l.closers = append(l.closers, readCloser)

		l.wg.Add(1)
		go func() {
			defer l.wg.Done()

			c := framework.GetConfig()
			excludePatterns := c.LogFilter
			excludeRegexpPatterns := c.RegexpLogFilter
			logStreamer := NewErrStreamer(excludePatterns, excludeRegexpPatterns)
			n, err := logStreamer.Stream(readCloser, ginkgo.GinkgoWriter)
			l.mu.Lock()
			defer l.mu.Unlock()
			if err != nil && !isExpectedStreamShutdownError(err) {
				l.resultErr = errors.Join(l.resultErr, err)
			}
			l.resultNum += n
		}()
	}
	return nil
}

func (l *LogChecker) Stop() error {
	// Close streams first so goroutines exit with "read on closed body" instead of server GOAWAY.
	for _, c := range l.closers {
		_ = c.Close()
	}
	l.wg.Wait()
	l.cancel()

	if l.resultErr != nil {
		return l.resultErr
	}
	if l.resultNum > 0 {
		return fmt.Errorf("%d error(s) have appeared in the `Virtualization-controller` logs (see test output above); add exclusions via logFilter/regexpLogFilter in e2e config if these are expected", l.resultNum)
	}

	return nil
}
