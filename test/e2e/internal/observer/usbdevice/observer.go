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

// Package usbdevice provides a USBDevice-specialized [observer.Observer]
// together with predicates ready to be used with its Never, Always and
// WaitFor primitives.
package usbdevice

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/observer"
)

// Observer is a convenience type alias for the generic Observer specialized
// for USBDevices.
type Observer = observer.Observer[*v1alpha2.USBDevice]

// Predicate is a convenience type alias for the generic Predicate specialized
// for USBDevices.
type Predicate = observer.Predicate[*v1alpha2.USBDevice]

// StartObserver starts a USBDevice Observer for the given device and
// registers a DeferCleanup that stops the underlying watch. The USBDevice is
// created by the controller once the NodeUSBDevice is assigned to a
// namespace, so start the observer before triggering the assignment to
// capture the creation event.
func StartObserver(ctx context.Context, f *framework.Framework, name, namespace string) Observer {
	GinkgoHelper()

	obs, err := observer.New[*v1alpha2.USBDevice](
		ctx,
		f.VirtClient().USBDevices(namespace),
		name,
		namespace,
	)
	Expect(err).NotTo(HaveOccurred(), "failed to start observer for USBDevice %s/%s", namespace, name)

	DeferCleanup(obs.Stop)

	return obs
}

// Exist reports the USBDevice has been observed at all: any event for the
// watched name means the controller has created the resource. Intended for
// use with [Observer.WaitFor].
func Exist() Predicate {
	return func(_ *v1alpha2.USBDevice) (bool, error) {
		return true, nil
	}
}
