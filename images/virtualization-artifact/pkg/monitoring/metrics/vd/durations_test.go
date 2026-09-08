/*
Copyright 2026 Flant JSC

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

package vd

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func stringReader(s string) *strings.Reader { return strings.NewReader(s) }

func TestObserveProvisioningStage(t *testing.T) {
	ProvisioningDuration.Reset()

	ObserveProvisioningStage(ProvisioningStageProvisioning, DataSourceHTTP, 17*time.Second)
	ObserveProvisioningStage(ProvisioningStageProvisioning, DataSourceHTTP, 43*time.Second)

	if got := testutil.CollectAndCount(ProvisioningDuration); got != 1 {
		t.Fatalf("expected a single series for one stage, got %d", got)
	}

	expected := `
# HELP d8_virtualization_virtualdisk_provisioning_duration_seconds The time a virtual disk spent in a provisioning stage.
# TYPE d8_virtualization_virtualdisk_provisioning_duration_seconds histogram
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="http",stage="provisioning",le="1"} 0
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="http",stage="provisioning",le="2"} 0
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="http",stage="provisioning",le="5"} 0
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="http",stage="provisioning",le="10"} 0
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="http",stage="provisioning",le="30"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="http",stage="provisioning",le="60"} 2
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="http",stage="provisioning",le="300"} 2
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="http",stage="provisioning",le="900"} 2
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="http",stage="provisioning",le="1800"} 2
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="http",stage="provisioning",le="3600"} 2
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="http",stage="provisioning",le="10800"} 2
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="http",stage="provisioning",le="43200"} 2
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="http",stage="provisioning",le="86400"} 2
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="http",stage="provisioning",le="+Inf"} 2
d8_virtualization_virtualdisk_provisioning_duration_seconds_sum{datasource="http",stage="provisioning"} 60
d8_virtualization_virtualdisk_provisioning_duration_seconds_count{datasource="http",stage="provisioning"} 2
`
	err := testutil.CollectAndCompare(ProvisioningDuration, stringReader(expected),
		"d8_virtualization_virtualdisk_provisioning_duration_seconds")
	if err != nil {
		t.Fatal(err)
	}
}

func TestObserveProvisioningStageCountsZeroSkipsNegative(t *testing.T) {
	ProvisioningDuration.Reset()

	ObserveProvisioningStage(ProvisioningStageWaitingForDependencies, DataSourceBlank, 0)
	ObserveProvisioningStage(ProvisioningStageWaitingForDependencies, DataSourceBlank, -time.Second)

	expected := `
# HELP d8_virtualization_virtualdisk_provisioning_duration_seconds The time a virtual disk spent in a provisioning stage.
# TYPE d8_virtualization_virtualdisk_provisioning_duration_seconds histogram
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="blank",stage="waiting_for_dependencies",le="1"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="blank",stage="waiting_for_dependencies",le="2"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="blank",stage="waiting_for_dependencies",le="5"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="blank",stage="waiting_for_dependencies",le="10"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="blank",stage="waiting_for_dependencies",le="30"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="blank",stage="waiting_for_dependencies",le="60"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="blank",stage="waiting_for_dependencies",le="300"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="blank",stage="waiting_for_dependencies",le="900"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="blank",stage="waiting_for_dependencies",le="1800"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="blank",stage="waiting_for_dependencies",le="3600"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="blank",stage="waiting_for_dependencies",le="10800"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="blank",stage="waiting_for_dependencies",le="43200"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="blank",stage="waiting_for_dependencies",le="86400"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_bucket{datasource="blank",stage="waiting_for_dependencies",le="+Inf"} 1
d8_virtualization_virtualdisk_provisioning_duration_seconds_sum{datasource="blank",stage="waiting_for_dependencies"} 0
d8_virtualization_virtualdisk_provisioning_duration_seconds_count{datasource="blank",stage="waiting_for_dependencies"} 1
`
	err := testutil.CollectAndCompare(ProvisioningDuration, stringReader(expected),
		"d8_virtualization_virtualdisk_provisioning_duration_seconds")
	if err != nil {
		t.Fatal(err)
	}
}

func TestObserveProvisioningStageSplitsByDataSource(t *testing.T) {
	ProvisioningDuration.Reset()

	ObserveProvisioningStage(ProvisioningStageProvisioning, DataSourceContainerImage, 11*time.Minute)
	ObserveProvisioningStage(ProvisioningStageProvisioning, DataSourceBlank, 2*time.Second)

	if got := testutil.CollectAndCount(ProvisioningDuration); got != 2 {
		t.Fatalf("expected a series per data source, got %d", got)
	}
}

func TestDataSourceLabel(t *testing.T) {
	tests := []struct {
		name   string
		source *v1alpha2.VirtualDiskDataSource
		want   string
	}{
		{"no source at all is a blank disk", nil, DataSourceBlank},
		{"http", &v1alpha2.VirtualDiskDataSource{Type: v1alpha2.DataSourceTypeHTTP}, DataSourceHTTP},
		{"container image", &v1alpha2.VirtualDiskDataSource{Type: v1alpha2.DataSourceTypeContainerImage}, DataSourceContainerImage},
		{"object ref", &v1alpha2.VirtualDiskDataSource{Type: v1alpha2.DataSourceTypeObjectRef}, DataSourceObjectRef},
		{"upload", &v1alpha2.VirtualDiskDataSource{Type: v1alpha2.DataSourceTypeUpload}, DataSourceUpload},
		{"a type from a newer api", &v1alpha2.VirtualDiskDataSource{Type: "SomethingElse"}, DataSourceUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DataSourceLabel(tt.source); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
