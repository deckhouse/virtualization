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

package precheck

import (
	"context"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

const (
	precreatedCVIPrecheckEnvName = "PRECREATED_CVI_PRECHECK"
)

// precreatedCVIPrecheck implements Precheck interface for precreated ClusterVirtualImages.
// This is a common precheck that creates or verifies precreated CVIs for the e2e suite.
type precreatedCVIPrecheck struct{}

func (p *precreatedCVIPrecheck) Label() string {
	return PrecheckPrecreatedCVI
}

func (p *precreatedCVIPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(precreatedCVIPrecheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("Precreated CVI precheck is disabled.\n"))
		return nil
	}

	if err := p.validateCleanupEnv(); err != nil {
		return err
	}

	cvis := object.PrecreatedClusterVirtualImages()
	By(fmt.Sprintf("Ensuring %d precreated CVIs are available", len(cvis)))

	if err := p.ensureCVIs(ctx, f, cvis); err != nil {
		return err
	}

	// Wait for all CVIs to become ready
	By(fmt.Sprintf("Waiting for all %d precreated CVIs to be ready", len(cvis)))
	p.waitForCVIsReady(cvis)

	By(fmt.Sprintf("All %d precreated CVIs are ready", len(cvis)))
	return nil
}

func (p *precreatedCVIPrecheck) validateCleanupEnv() error {
	env := os.Getenv(config.PrecreatedCVICleanupEnv)
	switch env {
	case "", "yes", "no":
		// valid values
	default:
		return fmt.Errorf("invalid value for %s env: %q (allowed: \"\", \"yes\", \"no\")", config.PrecreatedCVICleanupEnv, env)
	}
	return nil
}

func (p *precreatedCVIPrecheck) ensureCVIs(ctx context.Context, f *framework.Framework, cvis []*v1alpha2.ClusterVirtualImage) error {
	k8sClient := f.GenericClient()

	for _, cvi := range cvis {
		existing := &v1alpha2.ClusterVirtualImage{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: cvi.GetName()}, existing)

		if err == nil {
			// CVI already exists, verify it's ready
			if existing.Status.Phase != v1alpha2.ImageReady {
				_, _ = fmt.Fprintf(GinkgoWriter,
					"CVI %q exists but not ready (phase: %s), waiting...\n",
					cvi.GetName(), existing.Status.Phase)
			}
			continue
		}

		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to get CVI %q: %w", cvi.GetName(), err)
		}

		// CVI not found, create it
		_, _ = fmt.Fprintf(GinkgoWriter, "Creating CVI %q\n", cvi.GetName())

		err = k8sClient.Create(ctx, cvi)
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create CVI %q: %w", cvi.GetName(), err)
		}
	}
	return nil
}

func (p *precreatedCVIPrecheck) waitForCVIsReady(cvis []*v1alpha2.ClusterVirtualImage) {
	GinkgoHelper()

	// Convert []*ClusterVirtualImage to []client.Object for util.UntilObjectPhase
	objs := make([]client.Object, 0, len(cvis))
	for _, cvi := range cvis {
		objs = append(objs, cvi)
	}

	// Use util's polling with 5 minute timeout
	util.UntilObjectPhase(string(v1alpha2.ImageReady), framework.LongTimeout, objs...)
}

// Register precreatedCVI precheck as common (runs for all tests).
func init() {
	RegisterPrecheck(&precreatedCVIPrecheck{}, true)
}

// CleanupPrecreatedCVIs deletes precreated CVIs if PRECREATED_CVI_CLEANUP=yes.
func CleanupPrecreatedCVIs(ctx context.Context, f *framework.Framework) {
	GinkgoHelper()

	if !framework.GetConfig().IsPrecreatedCVICleanupNeeded {
		return
	}

	cvis := object.PrecreatedClusterVirtualImages()
	By(fmt.Sprintf("Cleaning up %d precreated CVIs", len(cvis)))

	k8sClient := f.GenericClient()
	for _, cvi := range cvis {
		err := k8sClient.Delete(ctx, cvi)
		if err != nil && !k8serrors.IsNotFound(err) {
			_, _ = fmt.Fprintf(GinkgoWriter, "Failed to delete CVI %q: %v\n", cvi.GetName(), err)
		}
	}
}
