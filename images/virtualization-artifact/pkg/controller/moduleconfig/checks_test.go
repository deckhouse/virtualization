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

package moduleconfig

import (
	"testing"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
)

func TestParseCIDRs(t *testing.T) {
	tests := []struct {
		name     string
		settings mcapi.SettingsValues
		wantLen  int
		wantErr  bool
	}{
		{name: "nil settings", settings: nil, wantLen: 0},
		{name: "missing key", settings: mcapi.SettingsValues{"dvcr": map[string]any{}}, wantLen: 0},
		{name: "nil value", settings: mcapi.SettingsValues{"virtualMachineCIDRs": nil}, wantLen: 0},
		{name: "invalid type", settings: mcapi.SettingsValues{"virtualMachineCIDRs": []string{"10.0.0.0/24"}}, wantErr: true},
		{name: "invalid cidr", settings: mcapi.SettingsValues{"virtualMachineCIDRs": []any{"bad"}}, wantErr: true},
		{name: "valid cidrs", settings: mcapi.SettingsValues{"virtualMachineCIDRs": []any{"10.0.0.0/24", "10.0.1.0/24"}}, wantLen: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cidrs, err := ParseCIDRs(tt.settings)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cidrs) != tt.wantLen {
				t.Fatalf("expected %d cidrs, got %d", tt.wantLen, len(cidrs))
			}
		})
	}
}
