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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

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

// TODO add auto generated mock.
type MockIPAM struct{}

func (m *MockIPAM) IsBound(_ string, _ *v1alpha2.VirtualMachineIPAddress) bool {
	return true
}

func (m *MockIPAM) CheckIPAddressAvailableForBinding(_ string, _ *v1alpha2.VirtualMachineIPAddress) error {
	return nil
}

func (m *MockIPAM) CreateIPAddress(_ context.Context, _ *v1alpha2.VirtualMachine, _ client.Client) error {
	return nil
}
