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

package config

import (
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Kubeconfig        string          `yaml:"kubeconfigBase64" env:"KUBECONFIG_BASE64" env-required:""`
	ResourcesPrefix   string          `yaml:"resourcesPrefix" env:"RESOURCES_PREFIX" env-default:"performance"`
	Namespace         string          `yaml:"namespace" env:"NAMESPACE" env-default:"default"`
	Interval          time.Duration   `yaml:"interval" env:"INTERVAL" env-default:"5s"`
	Count             int             `yaml:"count" env:"COUNT"`
	Debug             bool            `yaml:"debug" env:"DEBUG" env-default:"false"`
	Drainer           DrainerFeature  `yaml:"drainer"`
	Creator           CreatorFeature  `yaml:"creator"`
	Deleter           DeleterFeature  `yaml:"deleter"`
	Modifier          ModifierFeature `yaml:"modifier"`
	Nothing           NothingFeature  `yaml:"nothing"`
	ForceInterruption bool            `yaml:"forceInterruption"`
}

type DrainerFeature struct {
	Enabled       bool          `yaml:"enabled" env:"DRAINER_ENABLED" env-default:"false"`
	Once          bool          `yaml:"once" env:"DRAINER_ONCE" env-default:"false"`
	LabelSelector string        `yaml:"labelSelector" env:"DRAINER_LABEL_SELECTOR" env-default:"!node-role.kubernetes.io/master"`
	Interval      time.Duration `yaml:"interval" env:"DRAINER_INTERVAL" env-default:"1s"`
}

type CreatorFeature struct {
	Enabled  bool          `yaml:"enabled" env:"CREATOR_ENABLED" env-default:"false"`
	Interval time.Duration `yaml:"interval" env:"CREATOR_INTERVAL" env-default:"1s"`
}

type DeleterFeature struct {
	Enabled bool `yaml:"enabled" env:"DELETER_ENABLED" env-default:"false"`
	Weight  int  `yaml:"weight" env:"DELETER_WIGHT" env-default:"1"`
}

type ModifierFeature struct {
	Enabled bool `yaml:"enabled" env:"MODIFIER_ENABLED" env-default:"false"`
	Weight  int  `yaml:"weight" env:"MODIFIER_WIGHT" env-default:"1"`
}

type NothingFeature struct {
	Enabled bool `yaml:"enabled" env:"NOTHING_ENABLED" env-default:"false"`
	Weight  int  `yaml:"weight" env:"NOTHING_WIGHT" env-default:"1"`
}

func New() (Config, error) {
	var config Config
	err := cleanenv.ReadConfig("config.yaml", &config)
	if err != nil {
		return Config{}, err
	}

	return config, nil
}
