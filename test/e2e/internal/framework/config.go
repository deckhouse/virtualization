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

package framework

import (
	"context"
	"sync"

	"github.com/deckhouse/virtualization/test/e2e/internal/config"
)

var (
	conf               *config.Config
	confMu             sync.RWMutex
	once               sync.Once
	storageClassesOnce sync.Once
	errStorageClasses  error
)

func onceLoadConfig() {
	once.Do(func() {
		c, err := config.GetConfig()
		if err != nil {
			panic(err)
		}
		setConfig(c)
	})
}

// InitStorageClasses reads the StorageClasses of the cluster into the config.
// Call it from SynchronizedBeforeSuite: a failure there is reported, while a panic
// during the spec tree construction stops the process and Ginkgo drops the results
// of all processes.
func InitStorageClasses() error {
	storageClassesOnce.Do(func() {
		onceLoadConfig()
		InitClients()

		confMu.Lock()
		defer confMu.Unlock()

		errStorageClasses = conf.SetStorageClasses(context.Background(), clients.client)
	})
	return errStorageClasses
}

// GetConfig does not read StorageClasses: AfterSuite needs only the flags, also
// when InitStorageClasses failed.
func GetConfig() *config.Config {
	onceLoadConfig()

	confMu.RLock()
	copied := *conf
	confMu.RUnlock()

	return &copied
}

// SetConfig sets the config.
//
// Deprecated: config is populated by framework initialization; legacy should not mutate it.
func SetConfig(c *config.Config) {
	setConfig(c)
}

func setConfig(c *config.Config) {
	confMu.Lock()
	conf = c
	confMu.Unlock()
}
