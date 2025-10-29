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
	"sync"

	"github.com/deckhouse/virtualization/test/e2e/internal/config"
)

var (
	conf *config.Config
	once sync.Once
)

func onceLoadConfig() {
	once.Do(func() {
		c, err := config.GetConfig()
		if err != nil {
			panic(err)
		}
		SetConfig(c)
	})
}

func GetConfig() *config.Config {
	copied := *conf
	return &copied
}

// SetConfig sets the config.
// this needs because we have some legacy, config mutating in the main test suite
// should be refactored in the future
func SetConfig(c *config.Config) {
	conf = c
}

func init() {
	onceLoadConfig()
}
