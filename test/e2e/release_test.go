//go:build upgrade

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

package e2e

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/deckhouse/virtualization/test/e2e/controller"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	. "github.com/deckhouse/virtualization/test/e2e/release"
)

// Run with "task run:upgrade"; the build tag keeps these specs out of "task run".
func TestUpgrade(t *testing.T) {
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	RegisterFailHandler(Fail)
	RunSpecs(t, "Module Upgrade Tests")
}

var _ = SynchronizedBeforeSuite(func() {
	controller.NewBeforeProcess1Body()
	if err := framework.InitStorageClasses(); err != nil {
		Fail("failed to read StorageClasses: " + err.Error())
	}

	f := framework.NewFramework("")
	precheck.RunByLabels(f, precheck.PrecheckVirtualization, precheck.PrecheckVMC, precheck.PrecheckUpgradeStorageClasses)

	switch GetPhase() {
	case PhasePreUpgrade:
		CreateScenarioResources(context.Background(), f)
	case PhasePostUpgrade:
		ExpectProjectExists(context.Background(), f)
	}
}, func() {
	if err := framework.InitStorageClasses(); err != nil {
		Fail("failed to read StorageClasses: " + err.Error())
	}
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	controller.NewAfterAllProcessBody()
})
