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

package controller

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	virtv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.Level(-10))))
})

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

type TestReconcilerOptions struct {
	KnownObjects   []client.Object
	RuntimeObjects []runtime.Object
}

func NewTestVMReconciler(opts TestReconcilerOptions) *two_phase_reconciler.ReconcilerCore[*VMReconcilerState] {
	s := scheme.Scheme
	_ = cdiv1.AddToScheme(s)
	_ = metav1.AddMetaToScheme(s)
	_ = v1alpha2.AddToScheme(s)
	_ = virtv1.AddToScheme(s)

	builder := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(opts.KnownObjects...).
		WithRuntimeObjects(opts.RuntimeObjects...)

	cl := builder.Build()
	rec := record.NewFakeRecorder(10)

	return two_phase_reconciler.NewReconcilerCore[*VMReconcilerState](
		&VMReconciler{
			ipam: &MockIPAM{},
		},
		NewVMReconcilerState,
		two_phase_reconciler.ReconcilerOptions{
			Client:   cl,
			Recorder: rec,
			Scheme:   s,
			Log:      vmControllerLog,
		})
}

// TODO add auto generated mock.
type MockIPAM struct{}

func (m *MockIPAM) IsBound(_ string, _ *v1alpha2.VirtualMachineIPAddressClaim) bool {
	return true
}

func (m *MockIPAM) CheckClaimAvailableForBinding(_ string, _ *v1alpha2.VirtualMachineIPAddressClaim) error {
	return nil
}

func (m *MockIPAM) CreateIPAddressClaim(_ context.Context, _ *v1alpha2.VirtualMachine, _ client.Client) error {
	return nil
}

func (m *MockIPAM) DeleteIPAddressClaim(_ context.Context, _ *v1alpha2.VirtualMachineIPAddressClaim, _ client.Client) error {
	return nil
}
