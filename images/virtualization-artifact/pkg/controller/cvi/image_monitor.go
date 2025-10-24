/*
Copyright 2025 Flant JSC

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

package cvi

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/gc"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
)

const imageMonitorControllerName = "cvi-image-monitor-controller"

func SetupImageMonitorGC(
	mgr manager.Manager,
	log *log.Logger,
	monitorSettings config.BaseGcSettings,
	dvcrSettings *dvcr.Settings,
) error {
	mgrClient := mgr.GetClient()
	imageMon := newImageMonitorManager(mgrClient, dvcrSettings)
	source, err := gc.NewCronSource(monitorSettings.Schedule, imageMon, log.With("resource", "cvi"))
	if err != nil {
		return err
	}

	return gc.SetupGcController(imageMonitorControllerName,
		mgr,
		log,
		source,
		imageMon,
	)
}

func newImageMonitorManager(client client.Client, dvcrSettings *dvcr.Settings) *imageMonitorManager {
	return &imageMonitorManager{
		client:       client,
		dvcrSettings: dvcrSettings,
	}
}

type imageMonitorManager struct {
	client       client.Client
	dvcrSettings *dvcr.Settings
}

func (m *imageMonitorManager) New() client.Object {
	return &v1alpha2.ClusterVirtualImage{}
}

func (m *imageMonitorManager) ShouldBeDeleted(obj client.Object) bool {
	cvi, ok := obj.(*v1alpha2.ClusterVirtualImage)
	if !ok {
		return false
	}

	// Don't delete, just check if we should update status
	// Return false always since we're not deleting, just monitoring
	_ = cvi
	return false
}

func (m *imageMonitorManager) ListForDelete(ctx context.Context, now time.Time) ([]client.Object, error) {
	cviList := &v1alpha2.ClusterVirtualImageList{}
	err := m.client.List(ctx, cviList)
	if err != nil {
		return nil, err
	}

	objs := make([]client.Object, 0)

	for i := range cviList.Items {
		cvi := &cviList.Items[i]

		// Only check Ready images (CVI always uses DVCR)
		if cvi.Status.Phase != v1alpha2.ImageReady {
			continue
		}

		registryURL := cvi.Status.Target.RegistryURL
		if registryURL == "" {
			continue
		}

		// Check if image exists in DVCR
		exists, err := m.checkImage(ctx, registryURL)
		if err != nil {
			// Log error but continue checking other images
			continue
		}

		// If image is missing, mark for reconciliation to update status
		if !exists {
			// Update status immediately
			if err := m.markImageLost(ctx, cvi, registryURL); err != nil {
				// Log but continue
				continue
			}
			// Add to list for GC reconciler (though we're not deleting)
			objs = append(objs, cvi)
		}
	}

	return objs, nil
}

func (m *imageMonitorManager) checkImage(ctx context.Context, registryURL string) (bool, error) {
	username, password, err := m.getAuthCredentials(ctx)
	if err != nil {
		return false, err
	}

	insecure := m.dvcrSettings.InsecureTLS == "true"
	checker := dvcr.NewImageChecker(username, password, insecure)

	return checker.CheckImageExists(ctx, registryURL)
}

func (m *imageMonitorManager) markImageLost(ctx context.Context, cvi *v1alpha2.ClusterVirtualImage, registryURL string) error {
	cvi.Status.Phase = v1alpha2.ImageLost

	cb := conditions.NewConditionBuilder(cvicondition.ReadyType).
		Generation(cvi.Generation).
		Status(metav1.ConditionFalse).
		Reason(cvicondition.ImageLost).
		Message(fmt.Sprintf("Image %q not found in DVCR.", registryURL))

	conditions.SetCondition(cb, &cvi.Status.Conditions)

	return m.client.Status().Update(ctx, cvi)
}

func (m *imageMonitorManager) getAuthCredentials(ctx context.Context) (string, string, error) {
	if m.dvcrSettings.AuthSecret == "" {
		return "", "", nil
	}

	secret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      m.dvcrSettings.AuthSecret,
		Namespace: m.dvcrSettings.AuthSecretNamespace,
	}, secret)
	if err != nil {
		return "", "", fmt.Errorf("failed to get auth secret %s/%s: %w",
			m.dvcrSettings.AuthSecretNamespace, m.dvcrSettings.AuthSecret, err)
	}

	username := string(secret.Data["username"])
	password := string(secret.Data["password"])

	return username, password, nil
}
