package validators

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestIPAMValidatorValidateCreate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := mcapi.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme moduleconfig: %v", err)
	}
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha2: %v", err)
	}

	newValidator := func(settings map[string]any) *IPAMValidator {
		return NewIPAMValidator(fake.NewClientBuilder().WithScheme(scheme).WithObjects(&mcapi.ModuleConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "virtualization"},
			Spec:       mcapi.ModuleConfigSpec{Settings: settings},
		}).Build())
	}

	vm := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{VirtualMachineIPAddress: "vmip", Networks: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}}}}
	if _, err := newValidator(map[string]any{"dvcr": map[string]any{}}).ValidateCreate(t.Context(), vm); err == nil {
		t.Fatalf("expected error without CIDRs")
	}

	vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeNetwork, Name: "test"}}
	if _, err := newValidator(map[string]any{"dvcr": map[string]any{}, "virtualMachineCIDRs": []any{"10.0.0.0/24"}}).ValidateCreate(t.Context(), vm); err == nil {
		t.Fatalf("expected error without Main network")
	}

	vm.Spec.Networks = []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}}
	if _, err := newValidator(map[string]any{"dvcr": map[string]any{}, "virtualMachineCIDRs": []any{"10.0.0.0/24"}}).ValidateCreate(t.Context(), vm); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestIPAMValidatorValidateUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := mcapi.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme moduleconfig: %v", err)
	}
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme v1alpha2: %v", err)
	}

	validator := NewIPAMValidator(fake.NewClientBuilder().WithScheme(scheme).WithObjects(&mcapi.ModuleConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "virtualization"},
		Spec:       mcapi.ModuleConfigSpec{Settings: map[string]any{"dvcr": map[string]any{}}},
	}).Build())

	oldVM := &v1alpha2.VirtualMachine{}
	newVM := &v1alpha2.VirtualMachine{Spec: v1alpha2.VirtualMachineSpec{VirtualMachineIPAddress: "vmip", Networks: []v1alpha2.NetworksSpec{{Type: v1alpha2.NetworksTypeMain}}}}
	if _, err := validator.ValidateUpdate(t.Context(), oldVM, newVM); err == nil {
		t.Fatalf("expected error without CIDRs")
	}
}
