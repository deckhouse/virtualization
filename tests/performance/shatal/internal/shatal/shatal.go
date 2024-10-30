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

package shatal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/deckhouse/virtualization/shatal/internal/api"
	"github.com/deckhouse/virtualization/shatal/internal/config"
)

type Runner interface {
	Run(ctx context.Context)
}

type Shatal struct {
	api     *api.Client
	creator *Creator
	runners []Runner

	logger *slog.Logger
	exit   chan struct{}
	wg     sync.WaitGroup

	forceInterruption bool
}

func New(api *api.Client, conf config.Config, log *slog.Logger) (*Shatal, error) {
	shatal := Shatal{
		api:               api,
		logger:            log,
		exit:              make(chan struct{}),
		forceInterruption: conf.ForceInterruption,
	}

	nodes, err := api.GetNodes(context.Background(), conf.Drainer.LabelSelector)
	if err != nil {
		return nil, err
	}

	nodeLocks := make(map[string]*sync.Mutex)
	for _, node := range nodes {
		nodeLocks[node.Name] = &sync.Mutex{}
	}

	watcher := NewWatcher(api, conf.Interval, nodeLocks, log)
	shatal.runners = append(shatal.runners, watcher)

	if conf.Drainer.Enabled {
		if len(nodes) < 1 {
			return nil, errors.New("no node to drain")
		}

		shatal.logger.Info("With drainer", "selector", conf.Drainer.LabelSelector)

		drainer := NewDrainer(api, conf.Drainer.Interval, conf.Drainer.Once, nodeLocks, log)
		shatal.runners = append(shatal.runners, drainer)
	}

	if conf.Creator.Enabled {
		shatal.logger.Info("With creator")

		count := conf.Count
		if count == 0 {
			vms, err := api.GetVMs(context.Background())
			if err != nil {
				return nil, err
			}

			count = len(vms)
		}

		shatal.creator = NewCreator(api, conf.Namespace, conf.ResourcesPrefix, conf.Creator.Interval, count, log)
		shatal.runners = append(shatal.runners, shatal.creator)
	}

	if conf.Modifier.Enabled {
		shatal.logger.With("weight", conf.Modifier.Weight).Info("With modifier")

		modifier := NewModifier(api, conf.Namespace, log)
		watcher.Subscribe(modifier, conf.Modifier.Weight)
	}

	if conf.Deleter.Enabled {
		shatal.logger.With("weight", conf.Deleter.Weight).Info("With deleter")

		deleter := NewDeleter(api, log)
		watcher.Subscribe(deleter, conf.Deleter.Weight)
	}

	if conf.Nothing.Enabled {
		shatal.logger.With("weight", conf.Nothing.Weight).Info("With nothing")

		nothing := NewNothing(log)
		watcher.Subscribe(nothing, conf.Nothing.Weight)
	}

	return &shatal, nil
}

func (s *Shatal) Run() {
	s.logger.Info("Run")

	ctx, cancel := context.WithCancel(context.Background())

	for _, runner := range s.runners {
		r := runner

		s.wg.Add(1)
		go func() {
			r.Run(ctx)
			s.wg.Done()
		}()
	}

	go func() {
		select {
		case <-s.exit:
			s.logger.Info("Stop runners")
			if s.forceInterruption {
				fmt.Println("Performing force interruption")
				os.Exit(1)
			} else {
				cancel()
			}
		case <-ctx.Done():
		}
	}()
}

func (s *Shatal) Stop() {
	s.logger.Info("Stopping...")
	close(s.exit)

	s.wg.Wait()

	if s.creator != nil {
		s.logger.Info("Recover virtual machines up to target amount...")
		s.creator.createVMs(context.Background())
	}

	s.logger.Info("Stopped")
}
