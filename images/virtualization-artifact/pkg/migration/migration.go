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

package migration

import (
	"context"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

type constructor func(client client.Client, logger *log.Logger) (Migration, error)

var newMigrations = []constructor{
	newQEMUMaxLength36,
}

type Migration interface {
	Name() string
	Migrate(ctx context.Context) error
}

type Controller struct {
	client client.Client
	log    *log.Logger

	migrations []Migration
}

func NewController(client client.Client, log *log.Logger) (*Controller, error) {
	migrations := make([]Migration, len(newMigrations))
	for i, newMigration := range newMigrations {
		m, err := newMigration(client, log)
		if err != nil {
			return nil, err
		}
		migrations[i] = m
	}

	return &Controller{
		client:     client,
		log:        log,
		migrations: migrations,
	}, nil
}

func (c *Controller) Run(ctx context.Context) {
	wg := &sync.WaitGroup{}

	c.run(ctx, wg, c.migrations)
	wg.Wait()
}

func (c *Controller) run(ctx context.Context, wg *sync.WaitGroup, migrations []Migration) {
	for _, m := range migrations {
		if wg != nil {
			wg.Add(1)
		}
		lg := c.log.With("name", m.Name())
		lg.Info("Running migration")

		go func() {
			if wg != nil {
				defer wg.Done()
			}

			for {
				select {
				case <-ctx.Done():
					lg.Info("Cancelled migration")
					return
				default:
					if err := m.Migrate(ctx); err != nil {
						lg.Error("Failed to run migration, retry after 5s...", logger.SlogErr(err))
						time.Sleep(5 * time.Second)
						continue
					}
					lg.Info("Finished migration")
					return
				}
			}
		}()
	}
}
