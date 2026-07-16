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
