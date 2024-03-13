package server

import "sync"

type Runnable interface {
	Start()
	Stop()
	Done() chan struct{}
}

// RunnableGroup stops all runnables if one is stopped.
// It also waits until all done.
type RunnableGroup struct {
	runnables []Runnable

	stopOnce    sync.Once
	stopped     chan struct{}
	stopAllOnce sync.Once
}

func NewRunnableGroup() *RunnableGroup {
	return &RunnableGroup{
		runnables: make([]Runnable, 0),
		stopped:   make(chan struct{}),
	}
}

func (rg *RunnableGroup) Start() {
	// Start all.
	for i := range rg.runnables {
		r := rg.runnables[i]
		go func() {
			r.Start()
			rg.stopOnce.Do(func() {
				close(rg.stopped)
			})
		}()
	}

	// Stop all if one stopped.
	go func() {
		<-rg.stopped
		for i := range rg.runnables {
			rg.runnables[i].Stop()
		}
	}()

	// Wait until all Done.
	var wg sync.WaitGroup
	wg.Add(len(rg.runnables))
	for i := range rg.runnables {
		r := rg.runnables[i]
		go func() {
			<-r.Done()
			wg.Done()
		}()
	}
	wg.Wait()
}

// Add register another one runnable.
// Note: not designed for parallel registering.
func (rg *RunnableGroup) Add(r Runnable) {
	rg.runnables = append(rg.runnables, r)
}
