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

package service

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("BackoffService", func() {
	var svc *BackoffService

	BeforeEach(func() {
		svc = NewBackoffService()
	})

	newVM := func(name string) *v1alpha2.VirtualMachine {
		return &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
				UID:       types.UID("default/" + name),
			},
		}
	}

	newVD := func(name string) *v1alpha2.VirtualDisk {
		return &v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
				UID:       types.UID("default/" + name),
			},
		}
	}

	It("should return 0 failures and 0 backoff for new object", func() {
		vm := newVM("test-vm")
		Expect(svc.GetFailures(vm)).To(Equal(0))
		Expect(svc.Backoff(vm)).To(Equal(time.Duration(0)))
	})

	It("should increment failure count", func() {
		vm := newVM("test-vm")

		Expect(svc.RegisterFailure(vm)).To(Equal(1))
		Expect(svc.RegisterFailure(vm)).To(Equal(2))
		Expect(svc.GetFailures(vm)).To(Equal(2))
	})

	It("should reset failures", func() {
		vm := newVM("test-vm")

		svc.RegisterFailure(vm)
		svc.RegisterFailure(vm)
		svc.ResetFailures(vm)

		Expect(svc.GetFailures(vm)).To(Equal(0))
	})

	It("should track different objects of the same type independently", func() {
		vm1 := newVM("vm-1")
		vm2 := newVM("vm-2")

		svc.RegisterFailure(vm1)
		svc.RegisterFailure(vm1)
		svc.RegisterFailure(vm2)

		Expect(svc.GetFailures(vm1)).To(Equal(2))
		Expect(svc.GetFailures(vm2)).To(Equal(1))
	})

	It("should track different object types independently", func() {
		vm := newVM("obj-1")
		vd := newVD("obj-1")

		svc.RegisterFailure(vm)
		svc.RegisterFailure(vm)
		svc.RegisterFailure(vm)
		svc.RegisterFailure(vd)

		Expect(svc.GetFailures(vm)).To(Equal(3))
		Expect(svc.GetFailures(vd)).To(Equal(1))
	})

	It("should calculate exponential backoff", func() {
		vm := newVM("test-vm")

		// 1st failure: baseDelay * factor^0 = 2s
		Expect(svc.RegisterFailureAndBackoff(vm)).To(Equal(2 * time.Second))

		// 2nd failure: should be > 2s
		Expect(svc.RegisterFailureAndBackoff(vm)).To(BeNumerically(">", 2*time.Second))
	})

	It("should cap backoff at max delay", func() {
		vm := newVM("test-vm")

		for range 30 {
			svc.RegisterFailure(vm)
		}

		Expect(svc.Backoff(vm)).To(Equal(defaultMaxDelay))
	})

	It("should respect custom options", func() {
		custom := NewBackoffService(
			WithBaseDelay(5*time.Second),
			WithFactor(3.0),
			WithMaxDelay(1*time.Minute),
		)

		vm := newVM("test-vm")
		Expect(custom.RegisterFailureAndBackoff(vm)).To(Equal(5 * time.Second))

		for range 20 {
			custom.RegisterFailure(vm)
		}

		Expect(custom.Backoff(vm)).To(Equal(1 * time.Minute))
	})

	It("should return base delay for nil object", func() {
		Expect(svc.RegisterFailure(nil)).To(Equal(1))
		Expect(svc.GetFailures(nil)).To(Equal(0))
		Expect(svc.Backoff(nil)).To(Equal(time.Duration(0)))
		Expect(svc.RegisterFailureAndBackoff(nil)).To(Equal(defaultBaseDelay))
	})

	It("should be no-op reset for nil object", func() {
		Expect(func() { svc.ResetFailures(nil) }).NotTo(Panic())
	})
})
