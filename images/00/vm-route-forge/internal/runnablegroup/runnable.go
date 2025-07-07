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

package runnablegroup

import (
	"context"
	"errors"
	"sync"
)

type Runnable interface {
	Run(ctx context.Context) error
}

func NewRunnableGroup() *RunnableGroup {
	return &RunnableGroup{
		runnable: make([]Runnable, 0),
	}
}

type RunnableGroup struct {
	runnable  []Runnable
	startOnce sync.Once
	err       error
}

func (r *RunnableGroup) Add(runnable Runnable) {
	r.runnable = append(r.runnable, runnable)
}

func (r *RunnableGroup) Run(ctx context.Context) error {
	r.startOnce.Do(func() {
		r.err = r.run(ctx)
	})
	return r.err
}

func (r *RunnableGroup) run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wg := sync.WaitGroup{}
	var mu sync.Mutex
	var retErr error
	for _, runnable := range r.runnable {
		wg.Add(1)
		runnable := runnable
		go func() {
			defer wg.Done()
			if err := runnable.Run(ctx); err != nil {
				mu.Lock()
				retErr = errors.Join(retErr, err)
				mu.Unlock()
			}
			cancel()
		}()
	}
	wg.Wait()
	return retErr
}
