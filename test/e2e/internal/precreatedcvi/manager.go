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

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
)

const labelKey = "v12n-e2e-precreated"

// Manager runs bootstrap and cleanup of precreated CVIs for the e2e suite.
// The list of CVIs is loaded once during Bootstrap and reused in Cleanup.
type Manager struct {
	cvis []*v1alpha2.ClusterVirtualImage
}

// NewManager returns a new precreated CVI manager.
func NewManager() *Manager {
	return &Manager{}
}

// Bootstrap creates or reuses precreated CVIs in the cluster.
// Call once from SynchronizedBeforeSuite (process 1).
func (m *Manager) Bootstrap(ctx context.Context) error {
	m.cvis = object.PrecreatedClusterVirtualImages()

	for _, cvi := range m.cvis {
		if err := m.createOrReuse(ctx, cvi); err != nil {
			return fmt.Errorf("create or reuse CVI %q: %w", cvi.Name, err)
		}
	}

	return nil
}

// Cleanup deletes precreated CVIs.
// Call from SynchronizedAfterSuite (process 1). Uses the same CVI list as Bootstrap;
// if Bootstrap was not run, the list is loaded from object so that cleanup can still run.
func (m *Manager) Cleanup(ctx context.Context) error {
	if len(m.cvis) == 0 {
		m.cvis = object.PrecreatedClusterVirtualImages()
	}

	f := framework.NewFramework("")
	return f.Delete(ctx, m.CVIsAsObjects()...)
}

// CVIsAsObjects returns all managed CVIs as controller-runtime Objects.
func (m *Manager) CVIsAsObjects() []crclient.Object {
	objs := make([]crclient.Object, 0, len(m.cvis))
	for _, cvi := range m.cvis {
		objs = append(objs, cvi)
	}
	return objs
}

func (m *Manager) createOrReuse(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage) error {
	applyLabel(cvi)

	err := framework.GetClients().GenericClient().Create(ctx, cvi)
	if err == nil {
		return nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return err
	}
	return framework.GetClients().GenericClient().Get(ctx, crclient.ObjectKeyFromObject(cvi), cvi)
}

func applyLabel(cvi *v1alpha2.ClusterVirtualImage) {
	labels := cvi.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[labelKey] = "true"
	cvi.SetLabels(labels)
}
