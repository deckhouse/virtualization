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

// Package nodeusbdevice provides a NodeUSBDevice-specialized
// [observer.Observer] together with predicates ready to be used with its
// Never, Always and WaitFor primitives.
package nodeusbdevice

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/nodeusbdevicecondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

// Observer is a convenience type alias for the generic Observer specialized
// for NodeUSBDevices.
type Observer = observer.Observer[*v1alpha2.NodeUSBDevice]

// Predicate is a convenience type alias for the generic Predicate specialized
// for NodeUSBDevices.
type Predicate = observer.Predicate[*v1alpha2.NodeUSBDevice]

// StartObserver starts a NodeUSBDevice Observer (a cluster-scoped resource)
// for the given device and registers a DeferCleanup that stops the underlying
// watch. Start it before the action whose effect is being waited for, so the
// resulting status transitions are captured.
func StartObserver(ctx context.Context, f *framework.Framework, name string) Observer {
	GinkgoHelper()

	obs, err := observer.New[*v1alpha2.NodeUSBDevice](
		ctx,
		f.VirtClient().NodeUSBDevices(),
		name,
		"",
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for NodeUSBDevice %s", name)

	DeferCleanup(obs.Stop)

	return obs
}

// HaveAttachedCondition reports the NodeUSBDevice's Attached condition is
// present with the given status. Intended for use with [Observer.WaitFor].
func HaveAttachedCondition(status metav1.ConditionStatus) Predicate {
	return func(d *v1alpha2.NodeUSBDevice) (bool, error) {
		cond := meta.FindStatusCondition(d.Status.Conditions, string(nodeusbdevicecondition.AttachedType))
		if cond == nil {
			return false, nil
		}
		return cond.Status == status, nil
	}
}
