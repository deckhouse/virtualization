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
	"net/http"
	"os"
	"time"

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

var imageURLClient = &http.Client{Timeout: 30 * time.Second}

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
	p.waitForCVIsReady(ctx, cvis)

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
		// The HEAD request both checks that the source is reachable and tells the size
		// of the file behind the URL, which is what the CVI remembers about its source.
		sourceSize, probeErr := probeHTTPSource(ctx, cvi)

		existing := &v1alpha2.ClusterVirtualImage{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: cvi.GetName()}, existing)
		switch {
		case err == nil && !isStale(existing, cvi, sourceSize):
			if existing.Status.Phase != v1alpha2.ImageReady {
				_, _ = fmt.Fprintf(GinkgoWriter, "CVI %q exists but not ready (phase: %s), waiting...\n", cvi.GetName(), existing.Status.Phase)
			}
			continue
		case err == nil:
			if probeErr != nil {
				return probeErr
			}
			_, _ = fmt.Fprintf(GinkgoWriter, "CVI %q is stale (phase %s, recorded source size %q, current %q), recreating it...\n",
				cvi.GetName(), existing.Status.Phase, existing.GetAnnotations()[annSourceSize], sourceSize)
			if err := k8sClient.Delete(ctx, existing); err != nil && !k8serrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete CVI %q: %w", cvi.GetName(), err)
			}
			util.UntilObjectsDeleted(ctx, framework.ShortTimeout, existing)
		case !k8serrors.IsNotFound(err):
			return fmt.Errorf("failed to get CVI %q: %w", cvi.GetName(), err)
		}

		if probeErr != nil {
			return probeErr
		}
		if sourceSize != "" {
			cvi.SetAnnotations(map[string]string{annSourceSize: sourceSize})
		}
		_, _ = fmt.Fprintf(GinkgoWriter, "Creating CVI %q\n", cvi.GetName())
		if err := k8sClient.Create(ctx, cvi); err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create CVI %q: %w", cvi.GetName(), err)
		}
	}
	return nil
}

// annSourceSize records, on a precreated CVI, the Content-Length of the file it
// was imported from, so a later run notices that the file behind the same URL
// was replaced.
const annSourceSize = "e2e.virtualization.deckhouse.io/source-size"

// isStale reports that the existing precreated CVI cannot serve the suite: a lost
// DVCR image and a failed import never recover, and a different source URL or a
// different size of the file behind it means the image is no longer the one the
// suite expects. An unknown size ("") is not compared.
func isStale(existing, desired *v1alpha2.ClusterVirtualImage, sourceSize string) bool {
	if existing.Status.Phase == v1alpha2.ImageLost || existing.Status.Phase == v1alpha2.ImageFailed {
		return true
	}
	want := desired.Spec.DataSource.HTTP
	if want == nil {
		return false
	}
	have := existing.Spec.DataSource.HTTP
	if have == nil || have.URL != want.URL {
		return true
	}
	return sourceSize != "" && existing.GetAnnotations()[annSourceSize] != sourceSize
}

// probeHTTPSource checks that the image behind the CVI's HTTP source is
// available and returns its Content-Length. A CVI without an HTTP source yields
// an empty size and no error.
func probeHTTPSource(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) (string, error) {
	src := cvi.Spec.DataSource.HTTP
	if src == nil {
		return "", nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, src.URL, nil)
	if err != nil {
		return "", fmt.Errorf("CVI %q: %w", cvi.GetName(), err)
	}

	resp, err := imageURLClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("CVI %q: image is not available: %w", cvi.GetName(), err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("CVI %q: image %s is not available: HTTP %d", cvi.GetName(), src.URL, resp.StatusCode)
	}
	return resp.Header.Get("Content-Length"), nil
}

func (p *precreatedCVIPrecheck) waitForCVIsReady(ctx context.Context, cvis []*v1alpha2.ClusterVirtualImage) {
	GinkgoHelper()

	// Convert []*ClusterVirtualImage to []client.Object for util.UntilObjectPhase
	objs := make([]client.Object, 0, len(cvis))
	for _, cvi := range cvis {
		objs = append(objs, cvi)
	}

	// Temporarily increasing the system readiness timeout to 20 minutes; currently, the Ubuntu ISO image is taking a long time to load.
	// TODO: Revert back when the slow loading issue is fixed.
	util.UntilObjectPhase(ctx, string(v1alpha2.ImageReady), 1200*time.Second, objs...)
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
