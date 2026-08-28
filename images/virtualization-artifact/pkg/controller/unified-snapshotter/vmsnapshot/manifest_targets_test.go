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

package vmsnapshot

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const testNamespace = "ns"

func newTestScheme(t *testing.T) *apiruntime.Scheme {
	t.Helper()
	scheme := apiruntime.NewScheme()
	for _, f := range []func(*apiruntime.Scheme) error{
		clientgoscheme.AddToScheme,
		v1alpha2.AddToScheme,
	} {
		if err := f(scheme); err != nil {
			t.Fatal(err)
		}
	}
	return scheme
}

func newTestReconciler(t *testing.T, objs ...client.Object) *Reconciler {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(newTestScheme(t)).WithObjects(objs...).Build()
	return &Reconciler{Client: c, APIReader: c}
}

func TestPlanManifestTargetsAlwaysIncludesVM(t *testing.T) {
	vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace}}
	vms := &v1alpha2.VirtualMachineSnapshot{}
	r := newTestReconciler(t)

	targets, err := r.planManifestTargets(context.Background(), vms, vm)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Kind != v1alpha2.VirtualMachineKind || targets[0].Name != "vm1" {
		t.Fatalf("expected only the VM target, got %+v", targets)
	}
}

func TestPlanVMIPTarget(t *testing.T) {
	cases := []struct {
		name         string
		keep         v1alpha2.KeepIPAddress
		vmipType     v1alpha2.VirtualMachineIPAddressType
		namedInSpec  bool
		wantIncluded bool
	}{
		{"never+static", v1alpha2.KeepIPAddressNever, v1alpha2.VirtualMachineIPAddressTypeStatic, false, true},
		{"never+namedAuto", v1alpha2.KeepIPAddressNever, v1alpha2.VirtualMachineIPAddressTypeAuto, true, true},
		{"never+anonymousAuto", v1alpha2.KeepIPAddressNever, v1alpha2.VirtualMachineIPAddressTypeAuto, false, false},
		{"always+anonymousAuto", v1alpha2.KeepIPAddressAlways, v1alpha2.VirtualMachineIPAddressTypeAuto, false, true},
		{"always+static", v1alpha2.KeepIPAddressAlways, v1alpha2.VirtualMachineIPAddressTypeStatic, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vmip := &v1alpha2.VirtualMachineIPAddress{
				ObjectMeta: metav1.ObjectMeta{Name: "vmip1", Namespace: testNamespace},
				Spec:       v1alpha2.VirtualMachineIPAddressSpec{Type: tc.vmipType},
			}
			vm := &v1alpha2.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
				Status:     v1alpha2.VirtualMachineStatus{VirtualMachineIPAddress: "vmip1"},
			}
			if tc.namedInSpec {
				vm.Spec.VirtualMachineIPAddress = "vmip1"
			}
			vms := &v1alpha2.VirtualMachineSnapshot{Spec: v1alpha2.VirtualMachineSnapshotSpec{KeepIPAddress: tc.keep}}
			r := newTestReconciler(t, vmip)

			target, err := r.planVMIPTarget(context.Background(), vms, vm)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantIncluded && target == nil {
				t.Fatal("expected VMIP target, got nil")
			}
			if !tc.wantIncluded && target != nil {
				t.Fatalf("expected no VMIP target, got %+v", target)
			}
		})
	}
}

func TestPlanVMIPTargetSkippedWhenVMHasNone(t *testing.T) {
	vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace}}
	vms := &v1alpha2.VirtualMachineSnapshot{}
	r := newTestReconciler(t)

	target, err := r.planVMIPTarget(context.Background(), vms, vm)
	if err != nil || target != nil {
		t.Fatalf("expected no target and no error, got %+v, %v", target, err)
	}
}

func TestPlanVMIPTargetNotFound(t *testing.T) {
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
		Status:     v1alpha2.VirtualMachineStatus{VirtualMachineIPAddress: "missing"},
	}
	vms := &v1alpha2.VirtualMachineSnapshot{}
	r := newTestReconciler(t)

	_, err := r.planVMIPTarget(context.Background(), vms, vm)
	if !errors.Is(err, errManifestTargetNotReady) {
		t.Fatalf("expected errManifestTargetNotReady, got %v", err)
	}
}

func TestPlanVMMACTargetsSkipsMainNetwork(t *testing.T) {
	mac := &v1alpha2.VirtualMachineMACAddress{ObjectMeta: metav1.ObjectMeta{Name: "mac1", Namespace: testNamespace}}
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineStatus{
			Networks: []v1alpha2.NetworksStatus{
				{Type: v1alpha2.NetworksTypeMain, VirtualMachineMACAddressName: "should-be-ignored"},
				{Type: v1alpha2.NetworksTypeNetwork, VirtualMachineMACAddressName: "mac1"},
			},
		},
	}
	r := newTestReconciler(t, mac)

	targets, err := r.planVMMACTargets(context.Background(), vm)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Name != "mac1" || targets[0].Kind != v1alpha2.VirtualMachineMACAddressKind {
		t.Fatalf("expected exactly the secondary-network MAC, got %+v", targets)
	}
}

func TestPlanVMMACTargetsNotFound(t *testing.T) {
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineStatus{
			Networks: []v1alpha2.NetworksStatus{
				{Type: v1alpha2.NetworksTypeNetwork, VirtualMachineMACAddressName: "missing"},
			},
		},
	}
	r := newTestReconciler(t)

	_, err := r.planVMMACTargets(context.Background(), vm)
	if !errors.Is(err, errManifestTargetNotReady) {
		t.Fatalf("expected errManifestTargetNotReady, got %v", err)
	}
}

func TestPlanProvisionerSecretTarget(t *testing.T) {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "cloud-init", Namespace: testNamespace}}

	t.Run("userDataRef", func(t *testing.T) {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
			Spec: v1alpha2.VirtualMachineSpec{Provisioning: &v1alpha2.Provisioning{
				Type:        v1alpha2.ProvisioningTypeUserDataRef,
				UserDataRef: &v1alpha2.UserDataRef{Kind: v1alpha2.UserDataRefKindSecret, Name: "cloud-init"},
			}},
		}
		r := newTestReconciler(t, secret)

		target, err := r.planProvisionerSecretTarget(context.Background(), vm)
		if err != nil {
			t.Fatal(err)
		}
		if target == nil || target.Name != "cloud-init" || target.Kind != "Secret" {
			t.Fatalf("expected the provisioner Secret target, got %+v", target)
		}
	})

	t.Run("sysprepRef", func(t *testing.T) {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
			Spec: v1alpha2.VirtualMachineSpec{Provisioning: &v1alpha2.Provisioning{
				Type:       v1alpha2.ProvisioningTypeSysprepRef,
				SysprepRef: &v1alpha2.SysprepRef{Kind: v1alpha2.SysprepRefKindSecret, Name: "cloud-init"},
			}},
		}
		r := newTestReconciler(t, secret)

		target, err := r.planProvisionerSecretTarget(context.Background(), vm)
		if err != nil {
			t.Fatal(err)
		}
		if target == nil || target.Name != "cloud-init" {
			t.Fatalf("expected the provisioner Secret target, got %+v", target)
		}
	})

	t.Run("noProvisioning", func(t *testing.T) {
		vm := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace}}
		r := newTestReconciler(t)

		target, err := r.planProvisionerSecretTarget(context.Background(), vm)
		if err != nil || target != nil {
			t.Fatalf("expected no target and no error, got %+v, %v", target, err)
		}
	})

	t.Run("inlineUserData", func(t *testing.T) {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
			Spec: v1alpha2.VirtualMachineSpec{Provisioning: &v1alpha2.Provisioning{
				Type:     v1alpha2.ProvisioningTypeUserData,
				UserData: "#cloud-config\n",
			}},
		}
		r := newTestReconciler(t)

		target, err := r.planProvisionerSecretTarget(context.Background(), vm)
		if err != nil || target != nil {
			t.Fatalf("expected no target and no error for inline UserData, got %+v, %v", target, err)
		}
	})

	t.Run("wrongKind", func(t *testing.T) {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
			Spec: v1alpha2.VirtualMachineSpec{Provisioning: &v1alpha2.Provisioning{
				Type:        v1alpha2.ProvisioningTypeUserDataRef,
				UserDataRef: &v1alpha2.UserDataRef{Kind: "ConfigMap", Name: "cloud-init"},
			}},
		}
		r := newTestReconciler(t, secret)

		if _, err := r.planProvisionerSecretTarget(context.Background(), vm); err == nil {
			t.Fatal("expected an error for a non-Secret userDataRef kind")
		}
	})

	t.Run("notFound", func(t *testing.T) {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
			Spec: v1alpha2.VirtualMachineSpec{Provisioning: &v1alpha2.Provisioning{
				Type:        v1alpha2.ProvisioningTypeUserDataRef,
				UserDataRef: &v1alpha2.UserDataRef{Kind: v1alpha2.UserDataRefKindSecret, Name: "missing"},
			}},
		}
		r := newTestReconciler(t)

		_, err := r.planProvisionerSecretTarget(context.Background(), vm)
		if !errors.Is(err, errManifestTargetNotReady) {
			t.Fatalf("expected errManifestTargetNotReady, got %v", err)
		}
	})
}

func TestPlanVMBDATargetsOnlyHotplugged(t *testing.T) {
	vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{ObjectMeta: metav1.ObjectMeta{Name: "vmbda1", Namespace: testNamespace}}
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineStatus{
			BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
				{Hotplugged: false, VirtualMachineBlockDeviceAttachmentName: ""},
				{Hotplugged: true, VirtualMachineBlockDeviceAttachmentName: "vmbda1"},
			},
		},
	}
	r := newTestReconciler(t, vmbda)

	targets, err := r.planVMBDATargets(context.Background(), vm)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Name != "vmbda1" || targets[0].Kind != v1alpha2.VirtualMachineBlockDeviceAttachmentKind {
		t.Fatalf("expected exactly the hotplugged VMBDA, got %+v", targets)
	}
}

func TestPlanVMBDATargetsNotFound(t *testing.T) {
	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
		Status: v1alpha2.VirtualMachineStatus{
			BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
				{Hotplugged: true, VirtualMachineBlockDeviceAttachmentName: "missing"},
			},
		},
	}
	r := newTestReconciler(t)

	_, err := r.planVMBDATargets(context.Background(), vm)
	if !errors.Is(err, errManifestTargetNotReady) {
		t.Fatalf("expected errManifestTargetNotReady, got %v", err)
	}
}

func TestPlanManifestTargetsCombinesEverything(t *testing.T) {
	vmip := &v1alpha2.VirtualMachineIPAddress{
		ObjectMeta: metav1.ObjectMeta{Name: "vmip1", Namespace: testNamespace},
		Spec:       v1alpha2.VirtualMachineIPAddressSpec{Type: v1alpha2.VirtualMachineIPAddressTypeStatic},
	}
	mac := &v1alpha2.VirtualMachineMACAddress{ObjectMeta: metav1.ObjectMeta{Name: "mac1", Namespace: testNamespace}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "cloud-init", Namespace: testNamespace}}
	vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{ObjectMeta: metav1.ObjectMeta{Name: "vmbda1", Namespace: testNamespace}}

	vm := &v1alpha2.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{Name: "vm1", Namespace: testNamespace},
		Spec: v1alpha2.VirtualMachineSpec{Provisioning: &v1alpha2.Provisioning{
			Type:        v1alpha2.ProvisioningTypeUserDataRef,
			UserDataRef: &v1alpha2.UserDataRef{Kind: v1alpha2.UserDataRefKindSecret, Name: "cloud-init"},
		}},
		Status: v1alpha2.VirtualMachineStatus{
			VirtualMachineIPAddress: "vmip1",
			Networks: []v1alpha2.NetworksStatus{
				{Type: v1alpha2.NetworksTypeNetwork, VirtualMachineMACAddressName: "mac1"},
			},
			BlockDeviceRefs: []v1alpha2.BlockDeviceStatusRef{
				{Hotplugged: true, VirtualMachineBlockDeviceAttachmentName: "vmbda1"},
			},
		},
	}
	vms := &v1alpha2.VirtualMachineSnapshot{Spec: v1alpha2.VirtualMachineSnapshotSpec{KeepIPAddress: v1alpha2.KeepIPAddressAlways}}
	r := newTestReconciler(t, vmip, mac, secret, vmbda)

	targets, err := r.planManifestTargets(context.Background(), vms, vm)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"vm1":        v1alpha2.VirtualMachineKind,
		"vmip1":      v1alpha2.VirtualMachineIPAddressKind,
		"mac1":       v1alpha2.VirtualMachineMACAddressKind,
		"cloud-init": "Secret",
		"vmbda1":     v1alpha2.VirtualMachineBlockDeviceAttachmentKind,
	}
	if len(targets) != len(want) {
		t.Fatalf("expected %d targets, got %d: %+v", len(want), len(targets), targets)
	}
	for _, tgt := range targets {
		if want[tgt.Name] != tgt.Kind {
			t.Fatalf("unexpected target %+v", tgt)
		}
	}
}
