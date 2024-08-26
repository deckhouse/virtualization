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
	"fmt"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common"
)

type GCSettings struct {
	VMOP BaseGcSettings
}

type BaseGcSettings struct {
	TTL      metav1.Duration `json:"ttl,omitempty"`
	Schedule string          `json:"schedule"`
}

func LoadGcSettings() (GCSettings, error) {
	var gcSettings GCSettings
	if v, ok := os.LookupEnv(common.GcVmopScheduleVar); ok {
		gcSettings.VMOP.Schedule = v
	}
	if v, ok := os.LookupEnv(common.GcVmopTtlVar); ok {
		t, err := time.ParseDuration(v)
		if err != nil {
			return gcSettings, fmt.Errorf("invalid GC settings: %w", err)
		}
		gcSettings.VMOP.TTL.Duration = t
	}
	return gcSettings, nil
}
