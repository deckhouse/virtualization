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
	"sync"

	"github.com/onsi/ginkgo/v2"
	"golang.org/x/net/http2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

// isExpectedStreamCloseError reports whether err is an expected outcome of closing a log stream:
// nil, context.Canceled, or only http2.GoAwayError (e.g. when wrapped in errors.joinError).
// These should not fail the test.
func isExpectedStreamCloseError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return true
	}
	for _, e := range collectAllErrors(err) {
		if e == nil || errors.Is(e, context.Canceled) {
			continue
		}
		var goAway *http2.GoAwayError
		if !errors.As(e, &goAway) {
			return false
		}
	}
	return true
}

// collectAllErrors returns all errors from the error tree (following Unwrap() error and Unwrap() []error).
func collectAllErrors(err error) []error {
	var out []error
	var visit func(error)
	visit = func(e error) {
		if e == nil {
			return
		}
		out = append(out, e)
		// Multi-error (e.g. errors.Join)
		if multi, ok := e.(interface{ Unwrap() []error }); ok {
			for _, child := range multi.Unwrap() {
				visit(child)
			}
			return
		}
		// Single unwrap
		if child := errors.Unwrap(e); child != nil {
			visit(child)
		}
	}
	visit(err)
	return out
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
			if err != nil && !errors.Is(err, context.Canceled) && !isExpectedStreamCloseError(err) {
				l.resultErr = errors.Join(l.resultErr, err)
			} else if err != nil && !errors.Is(err, context.Canceled) {
				ginkgo.GinkgoWriter.Printf("Warning! %v\n", err)
			}
			l.resultNum += n
		}()
	}
	return nil
}

func (l *LogChecker) Stop() error {
	l.cancel()
	l.wg.Wait()
	for _, c := range l.closers {
		_ = c.Close()
	}

	// Ignore errors that are only GOAWAY/canceled (connection closed during teardown).
	if l.resultErr != nil && !isExpectedStreamCloseError(l.resultErr) {
		return l.resultErr
	}
	if l.resultNum > 0 {
		return fmt.Errorf("errors have appeared in the `Virtualization-controller` logs")
	}

	return nil
}
