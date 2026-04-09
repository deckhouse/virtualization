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

package release

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/controller"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/precreatedcvi"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var cviManager = precreatedcvi.NewManager()

func TestRelease(t *testing.T) {
	log.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	RegisterFailHandler(Fail)
	RunSpecs(t, "Release Tests")
}

var _ = SynchronizedBeforeSuite(func() {
	controller.NewBeforeProcess1Body()

	By("Creating or reusing precreated CVIs")
	err := cviManager.Bootstrap(context.Background())
	Expect(err).NotTo(HaveOccurred())

	cvis := cviManager.CVIsAsObjects()
	By(fmt.Sprintf("Waiting for all %d precreated CVIs to be ready", len(cvis)))
	util.UntilObjectPhase(string(v1alpha2.ImageReady), framework.LongTimeout, cvis...)
}, func() {})

var _ = SynchronizedAfterSuite(func() {
	if !config.IsCleanUpNeeded() || !config.IsPrecreatedCVICleanupNeeded() {
		return
	}

	By("Cleaning up precreated CVIs")
	err := cviManager.Cleanup(context.Background())
	Expect(err).NotTo(HaveOccurred(), "Failed to delete precreated CVIs")
}, func() {
	controller.NewAfterAllProcessBody()
})
