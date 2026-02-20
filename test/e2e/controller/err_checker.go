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
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/onsi/ginkgo/v2"
	"golang.org/x/net/http2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const pollInterval = 1 * time.Second // tradeoff: smaller = less log lag, more API calls

// isExpectedShutdownError reports whether err is expected when we stop (cancel context):
// GOAWAY, connection reset, or context.Canceled. Such errors must not fail the test.
func isExpectedShutdownError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return true
	}
	var goAway *http2.GoAwayError
	if errors.As(err, &goAway) {
		return true
	}
	type multiUnwrap interface{ Unwrap() []error }
	if u, ok := err.(multiUnwrap); ok {
		for _, e := range u.Unwrap() {
			if isExpectedShutdownError(e) {
				return true
			}
		}
	}
	s := err.Error()
	if strings.Contains(s, "connection reset by peer") ||
		strings.Contains(s, "read on closed body") ||
		strings.Contains(s, "use of closed network connection") {
		return true
	}
	return false
}

// LogChecker detects `v12n-controller` errors while the test suite is running.
// It polls pod logs in short-lived requests (no Follow) so that Stop() only cancels
// the context; no long-lived streams, so no GOAWAY or "read on closed body" errors.
type LogChecker struct {
	ctx     context.Context
	cancel  context.CancelFunc
	wg      *sync.WaitGroup
	startAt time.Time

	resultNum int
	resultErr error
	mu        sync.Mutex
}

func (l *LogChecker) Start() error {
	l.ctx, l.cancel = context.WithCancel(context.Background())
	l.wg = &sync.WaitGroup{}
	l.startAt = time.Now()

	kubeClient := framework.GetClients().KubeClient()
	pods, err := kubeClient.CoreV1().Pods(VirtualizationNamespace).List(l.ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{"app": VirtualizationController}).String(),
	})
	if err != nil {
		return fmt.Errorf("failed to obtain the `Virtualization-controller` pods: %w", err)
	}

	c := framework.GetConfig()
	excludePatterns := c.LogFilter
	excludeRegexpPatterns := c.RegexpLogFilter

	for _, p := range pods.Items {
		podName := p.Name
		l.wg.Add(1)
		go func() {
			defer l.wg.Done()
			l.pollPodLogs(podName, excludePatterns, excludeRegexpPatterns)
		}()
	}
	return nil
}

func (l *LogChecker) pollPodLogs(podName string, excludePatterns []string, excludeRegexpPatterns []regexp.Regexp) {
	kubeClient := framework.GetClients().KubeClient()
	streamer := NewErrStreamer(excludePatterns, excludeRegexpPatterns)
	streamer.SetSince(l.startAt)
	sinceTime := l.startAt

	for {
		select {
		case <-l.ctx.Done():
			return
		default:
		}

		req := kubeClient.CoreV1().Pods(VirtualizationNamespace).GetLogs(podName, &corev1.PodLogOptions{
			Container:  VirtualizationController,
			SinceTime:  &metav1.Time{Time: sinceTime},
			Timestamps: true,
		})
		stream, err := req.Stream(l.ctx)
		if err != nil {
			if isExpectedShutdownError(err) {
				return
			}
			l.mu.Lock()
			l.resultErr = errors.Join(l.resultErr, fmt.Errorf("pod %s: %w", podName, err))
			l.mu.Unlock()
			l.sleepOrDone()
			continue
		}

		n, lastTime, streamErr := streamer.Stream(stream, ginkgo.GinkgoWriter)
		_ = stream.Close()
		if streamErr != nil && !isExpectedShutdownError(streamErr) {
			l.mu.Lock()
			l.resultErr = errors.Join(l.resultErr, fmt.Errorf("pod %s: %w", podName, streamErr))
			l.mu.Unlock()
		}
		if !lastTime.IsZero() {
			sinceTime = lastTime
		}
		l.mu.Lock()
		l.resultNum += n
		l.mu.Unlock()

		l.sleepOrDone()
	}
}

func (l *LogChecker) sleepOrDone() {
	t := time.NewTimer(pollInterval)
	defer t.Stop()
	select {
	case <-l.ctx.Done():
		return
	case <-t.C:
		return
	}
}

func (l *LogChecker) Stop() error {
	l.cancel()
	l.wg.Wait()

	if l.resultErr != nil {
		return l.resultErr
	}
	if l.resultNum > 0 {
		return fmt.Errorf("%d error(s) have appeared in the `Virtualization-controller` logs (see test output above); add exclusions via logFilter/regexpLogFilter in e2e config if these are expected", l.resultNum)
	}

	return nil
}
