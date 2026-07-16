package vmip

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/deckhouse/pkg/log"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestValidatorValidateCreateRequiresCIDRs(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := mcapi.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme moduleconfig: %v", err)
	}
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha2: %v", err)
	}

	newValidator := func(settings map[string]any) *Validator {
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&mcapi.ModuleConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "virtualization"},
			Spec:       mcapi.ModuleConfigSpec{Settings: settings},
		}).Build()
		ipService, err := service.NewIPAddressService([]string{"10.0.0.0/24"}, cli, nil)
		if err != nil {
			t.Fatalf("NewIPAddressService: %v", err)
		}
		return NewValidator(log.NewNop(), cli, ipService)
	}

	vmip := &v1alpha2.VirtualMachineIPAddress{Spec: v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeAuto}}
	if _, err := newValidator(map[string]any{"dvcr": map[string]any{}}).ValidateCreate(t.Context(), vmip); err == nil {
		t.Fatalf("expected error without CIDRs")
	}
	if _, err := newValidator(map[string]any{"dvcr": map[string]any{}, "virtualMachineCIDRs": []any{"10.0.0.0/24"}}).ValidateCreate(t.Context(), vmip); err != nil {
		t.Fatalf("expected success with CIDRs, got: %v", err)
	}
}
