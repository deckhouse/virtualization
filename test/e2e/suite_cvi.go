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

const precreatedCVILabel = "v12n-e2e-precreated"

// Additional labels to add to precreated CVIs (e.g., for test discovery)
var precreatedCVITestLabels = make(map[string]string)

func bootstrapPrecreatedCVIs() {
	GinkgoHelper()

	ctx := context.Background()
	for _, cvi := range object.PrecreatedClusterVirtualImages() {
		By(fmt.Sprintf("Create or reuse precreated CVI %q in the cluster", cvi.Name))
		created, err := createOrReusePrecreatedCVI(ctx, cvi)
		Expect(err).NotTo(HaveOccurred())
		if created {
			By(fmt.Sprintf("Precreated CVI %q has been created", cvi.Name))
			continue
		}
		By(fmt.Sprintf("Precreated CVI %q already exists and will be reused", cvi.Name))
	}

	By("Wait until all precreated CVIs are ready")
	util.UntilObjectPhase(string(v1alpha2.ImageReady), framework.LongTimeout, precreatedClusterVirtualImagesAsObjects()...)
	for _, cvi := range object.PrecreatedClusterVirtualImages() {
		By(fmt.Sprintf("Precreated CVI %q is ready", cvi.Name))
	}
}

// AddTestLabelsToPrecreatedCVI adds additional labels to precreated CVIs for test discovery.
// Must be called before applying kustomize in tests that use precreated CVIs.
func AddTestLabelsToPrecreatedCVI(labels map[string]string) {
	for k, v := range labels {
		precreatedCVITestLabels[k] = v
	}
}

func cleanupPrecreatedCVIs() {
	GinkgoHelper()

	if !config.IsCleanUpNeeded() {
		return
	}

	ctx := context.Background()
	for _, cvi := range object.PrecreatedClusterVirtualImages() {
		err := framework.GetClients().GenericClient().Delete(ctx, cvi)
		Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue(), "failed to delete precreated CVI %q: %v", cvi.Name, err)
	}
}

func createOrReusePrecreatedCVI(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (bool, error) {
	setPrecreatedCVILabel(cvi)

	err := framework.GetClients().GenericClient().Create(ctx, cvi)
	if err == nil {
		return true, nil
	}

	if !k8serrors.IsAlreadyExists(err) {
		return false, err
	}

	return false, framework.GetClients().GenericClient().Get(ctx, crclient.ObjectKeyFromObject(cvi), cvi)
}

func precreatedClusterVirtualImagesAsObjects() []crclient.Object {
	cvis := object.PrecreatedClusterVirtualImages()
	objs := make([]crclient.Object, 0, len(cvis))
	for _, cvi := range cvis {
		objs = append(objs, cvi)
	}

	return objs
}

func setPrecreatedCVILabel(cvi *v1alpha2.ClusterVirtualImage) {
	labels := cvi.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[precreatedCVILabel] = "true"
	// Add test-specific labels for test discovery
	for k, v := range precreatedCVITestLabels {
		labels[k] = v
	}
	cvi.SetLabels(labels)
}
