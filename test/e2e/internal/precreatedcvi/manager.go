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

// Package precreatedcvi provides suite-level lifecycle (bootstrap and cleanup) for
// precreated ClusterVirtualImages used by e2e tests.
package precreatedcvi

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const labelKey = "v12n-e2e-precreated"

// PrecreatedCVIManager runs bootstrap and cleanup of precreated CVIs for the e2e suite.
// The list of CVIs is loaded once during Bootstrap and reused in Cleanup.
type PrecreatedCVIManager struct {
	cvis []*v1alpha2.ClusterVirtualImage
}

// NewPrecreatedCVIManager returns a new precreated CVI manager.
func NewPrecreatedCVIManager() *PrecreatedCVIManager {
	return &PrecreatedCVIManager{}
}

// Bootstrap creates or reuses precreated CVIs in the cluster, then waits until all are ready.
// Call once from SynchronizedBeforeSuite (process 1).
func (m *PrecreatedCVIManager) Bootstrap(ctx context.Context) {
	GinkgoHelper()

	m.cvis = object.PrecreatedClusterVirtualImages()

	for _, cvi := range m.cvis {
		By(fmt.Sprintf("Create or reuse precreated CVI %q in the cluster", cvi.Name))
		created, err := m.createOrReuse(ctx, cvi)
		Expect(err).NotTo(HaveOccurred())
		if created {
			By(fmt.Sprintf("Precreated CVI %q has been created", cvi.Name))
		} else {
			By(fmt.Sprintf("Precreated CVI %q already exists and will be reused", cvi.Name))
		}
	}

	By("Wait until all precreated CVIs are ready")
	util.UntilObjectPhase(string(v1alpha2.ImageReady), framework.LongTimeout, m.cvisAsObjects()...)

	for _, cvi := range m.cvis {
		By(fmt.Sprintf("Precreated CVI %q is ready", cvi.Name))
	}
}

// Cleanup deletes precreated CVIs when both POST_CLEANUP and PRECREATED_CVI_CLEANUP allow it.
// Call from SynchronizedAfterSuite (process 1). Uses the same CVI list as Bootstrap; if Bootstrap
// was not run, the list is loaded from object so that cleanup can still run.
func (m *PrecreatedCVIManager) Cleanup(ctx context.Context) {
	GinkgoHelper()

	if !config.IsCleanUpNeeded() {
		return
	}
	if !config.IsPrecreatedCVICleanupNeeded() {
		return
	}

	if len(m.cvis) == 0 {
		m.cvis = object.PrecreatedClusterVirtualImages()
	}

	f := framework.NewFramework("")
	err := f.Delete(ctx, m.cvisAsObjects()...)
	Expect(err).NotTo(HaveOccurred(), "Failed to delete precreated CVIs")
}

func (m *PrecreatedCVIManager) createOrReuse(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (bool, error) {
	applyLabel(cvi)

	err := framework.GetClients().GenericClient().Create(ctx, cvi)
	if err == nil {
		return true, nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return false, err
	}
	return false, framework.GetClients().GenericClient().Get(ctx, crclient.ObjectKeyFromObject(cvi), cvi)
}

func (m *PrecreatedCVIManager) cvisAsObjects() []crclient.Object {
	objs := make([]crclient.Object, 0, len(m.cvis))
	for _, cvi := range m.cvis {
		objs = append(objs, cvi)
	}
	return objs
}

func applyLabel(cvi *v1alpha2.ClusterVirtualImage) {
	labels := cvi.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[labelKey] = "true"
	cvi.SetLabels(labels)
}
