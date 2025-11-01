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
			if err != nil && !errors.Is(err, context.Canceled) {
				// TODO: Find an alternative way to store Virtualization Controller errors without streaming.
				// `http2.GoAwayError` likely appears when the context is canceled and readers are closed.
				// It should not cause tests to fail.
				var goAwayError *http2.GoAwayError
				if errors.As(err, &goAwayError) {
					ginkgo.GinkgoWriter.Printf("Warning! %w\n", err)
				} else {
					l.resultErr = errors.Join(l.resultErr, err)
				}
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

	if l.resultErr != nil {
		return l.resultErr
	}
	if l.resultNum > 0 {
		return fmt.Errorf("errors have appeared in the `Virtualization-controller` logs")
	}

	return nil
}
