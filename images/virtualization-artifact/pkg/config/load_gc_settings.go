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
)

const (
	GcVmopTTLVar              = "GC_VMOP_TTL"
	GcVmopScheduleVar         = "GC_VMOP_SCHEDULE"
	GcVMIMigrationTTLVar      = "GC_VMI_MIGRATION_TTL"
	GcVMIMigrationScheduleVar = "GC_VMI_MIGRATION_SCHEDULE"
)

type GCSettings struct {
	VMOP         BaseGcSettings
	VMIMigration BaseGcSettings
}

type BaseGcSettings struct {
	TTL      metav1.Duration `json:"ttl,omitempty"`
	Schedule string          `json:"schedule"`
}

func LoadGcSettings() (GCSettings, error) {
	var gcSettings GCSettings
	base, err := GetBaseGCSettingsFromEnv(GcVmopScheduleVar, GcVmopTTLVar)
	if err != nil {
		return gcSettings, err
	}
	gcSettings.VMOP = base

	base, err = GetBaseGCSettingsFromEnv(GcVMIMigrationScheduleVar, GcVMIMigrationTTLVar)
	if err != nil {
		return gcSettings, err
	}
	gcSettings.VMIMigration = base

	return gcSettings, nil
}

func GetBaseGCSettingsFromEnv(envSchedule, envTTL string) (BaseGcSettings, error) {
	base := NewDefaultBaseGcSettings()
	if v, ok := os.LookupEnv(envSchedule); ok {
		base.Schedule = v
	}
	if v, ok := os.LookupEnv(envTTL); ok {
		t, err := time.ParseDuration(v)
		if err != nil {
			return BaseGcSettings{}, fmt.Errorf("invalid GC settings: %w", err)
		}
		if t == 0 {
			return BaseGcSettings{}, fmt.Errorf("invalid GC settings: TTL cannot be 0: %w", err)
		}
		base.TTL = metav1.Duration{Duration: t}
	}
	return base, nil
}

func NewDefaultBaseGcSettings() BaseGcSettings {
	return BaseGcSettings{
		TTL:      metav1.Duration{Duration: 24 * time.Hour * 7},
		Schedule: "0 0 * * *",
	}
}
