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

package usbdevice

import (
	"context"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/usbdevice/internal"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/client/generated/clientset/versioned"
)

const (
	ControllerName = "usbdevice-controller"
)

func NewController(
	ctx context.Context,
	mgr manager.Manager,
	log *log.Logger,
) (controller.Controller, error) {
	if !featuregates.Default().Enabled(featuregates.USB) {
		return nil, nil
	}

	recorder := eventrecord.NewEventRecorderLogger(mgr, ControllerName)
	client := mgr.GetClient()

	virtClient, err := versioned.NewForConfig(mgr.GetConfig())
	if err != nil {
		return nil, err
	}

	handlers := []Handler{
		internal.NewDeletionHandler(client, virtClient, recorder),
		internal.NewLifecycleHandler(client, mgr.GetScheme()),
	}

	r := NewReconciler(client, handlers...)

	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler:       r,
		RecoverPanic:     ptr.To(true),
		LogConstructor:   logger.NewConstructor(log),
		CacheSyncTimeout: 10 * time.Minute,
		UsePriorityQueue: ptr.To(true),
	})
	if err != nil {
		return nil, err
	}

	if err = r.SetupController(ctx, mgr, c); err != nil {
		return nil, err
	}

	log.Info("Initialized USBDevice controller")
	return c, nil
}
