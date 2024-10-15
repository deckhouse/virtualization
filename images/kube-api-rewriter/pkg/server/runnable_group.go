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

package server

import (
	"sync"
)

type Runnable interface {
	Start()
	Stop()
}

// RunnableGroup is a group of Runnables that should run until one of them stops.
type RunnableGroup struct {
	runnables []Runnable
}

func NewRunnableGroup() *RunnableGroup {
	return &RunnableGroup{
		runnables: make([]Runnable, 0),
	}
}

// Add register Runnable in a group.
// Note: not designed for parallel registering.
func (rg *RunnableGroup) Add(r Runnable) {
	rg.runnables = append(rg.runnables, r)
}

// Start starts all Runnables and stops all of them when at least one Runnable stops.
func (rg *RunnableGroup) Start() {
	// Start all runnables.
	oneStoppedCh := rg.startAll()

	// Block until one runnable is stopped.
	<-oneStoppedCh

	// Wait until all Runnables stop.
	rg.stopAll()
}

// startAll calls Start for each Runnable in separate go routines.
// It waits until all go routines starts.
// It returns a channel, so caller can receive event when one of the Runnables stops.
func (rg *RunnableGroup) startAll() chan struct{} {
	oneStopped := make(chan struct{})
	var closeOnce sync.Once

	for i := range rg.runnables {
		r := rg.runnables[i]
		go func() {
			r.Start()
			closeOnce.Do(func() {
				close(oneStopped)
			})
		}()
	}

	return oneStopped
}

// stopAll calls Stop for each Runnable in a separate go routine.
// It waits until all go routines starts.
func (rg *RunnableGroup) stopAll() {
	var wg sync.WaitGroup
	wg.Add(len(rg.runnables))
	for i := range rg.runnables {
		r := rg.runnables[i]
		go func() {
			r.Stop()
			wg.Done()
		}()
	}
	wg.Wait()
}
