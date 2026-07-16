package moduleconfig

import (
	"context"
	"net/netip"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appconfig "github.com/deckhouse/virtualization-controller/pkg/config"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestCIDRsValidatorValidateUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	podSubnet := netip.MustParsePrefix("10.111.0.0/16")
	serviceSubnet := netip.MustParsePrefix("10.222.0.0/16")

	validator := newCIDRsValidator(fake.NewClientBuilder().WithScheme(scheme).Build(), &appconfig.ClusterSubnets{
		PodSubnet:     podSubnet,
		ServiceSubnet: serviceSubnet,
	})

	newMC := &mcapi.ModuleConfig{Spec: mcapi.ModuleConfigSpec{Settings: mcapi.SettingsValues{"dvcr": map[string]any{}}}}
	if _, err := validator.ValidateUpdate(context.Background(), nil, newMC); err != nil {
		t.Fatalf("expected no error without CIDRs, got: %v", err)
	}

	newMC.Spec.Settings["virtualMachineCIDRs"] = []any{"10.111.0.0/24", "10.111.0.0/25"}
	if _, err := validator.ValidateUpdate(context.Background(), nil, newMC); err == nil {
		t.Fatalf("expected overlap validation error")
	}
}

func TestRemoveCIDRsValidatorValidateUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	oldNone := &mcapi.ModuleConfig{Spec: mcapi.ModuleConfigSpec{Settings: mcapi.SettingsValues{"dvcr": map[string]any{}}}}
	newNone := &mcapi.ModuleConfig{Spec: mcapi.ModuleConfigSpec{Settings: mcapi.SettingsValues{"dvcr": map[string]any{}}}}
	newSome := &mcapi.ModuleConfig{Spec: mcapi.ModuleConfigSpec{Settings: mcapi.SettingsValues{"dvcr": map[string]any{}, "virtualMachineCIDRs": []any{"10.0.0.0/24"}}}}
	oldSome := &mcapi.ModuleConfig{Spec: mcapi.ModuleConfigSpec{Settings: mcapi.SettingsValues{"dvcr": map[string]any{}, "virtualMachineCIDRs": []any{"10.0.0.0/24"}}}}

	validator := newRemoveCIDRsValidator(fake.NewClientBuilder().WithScheme(scheme).Build())
	for name, pair := range map[string]struct{ oldMC, newMC *mcapi.ModuleConfig }{
		"old none new none":                {oldNone, newNone},
		"old none new some":                {oldNone, newSome},
		"old some new none without leases": {oldSome, newNone},
		"old some new some same":           {oldSome, newSome},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validator.ValidateUpdate(context.Background(), pair.oldMC, pair.newMC); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	lease := &v1alpha2.VirtualMachineIPAddressLease{ObjectMeta: metav1.ObjectMeta{Name: "ip-10-0-0-10"}}
	validator = newRemoveCIDRsValidator(fake.NewClientBuilder().WithScheme(scheme).WithObjects(lease).Build())
	if _, err := validator.ValidateUpdate(context.Background(), oldSome, newNone); err == nil {
		t.Fatalf("expected lease protection error")
	}
}
