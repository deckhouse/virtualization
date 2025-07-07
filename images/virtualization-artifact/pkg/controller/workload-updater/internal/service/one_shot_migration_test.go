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

package service

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("TestOnceShotMigrationService", func() {
	const (
		vmName      = "vm"
		vmNamespace = "default"
	)

	newVM := func() *v1alpha2.VirtualMachine {
		return vmbuilder.NewEmpty(vmName, vmNamespace)
	}

	newKVVMI := func() *virtv1.VirtualMachineInstance {
		return &virtv1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vmName,
				Namespace: vmNamespace,
				Annotations: map[string]string{
					"key": "old-value",
				},
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualMachineInstance",
				APIVersion: virtv1.GroupVersion.String(),
			},
		}
	}

	It("Retry 10 times expect one migration", func() {
		prefix := "vmop-prefix-"

		fakeClient, err := testutil.NewFakeClientWithObjects(newVM(), newKVVMI())
		Expect(err).ToNot(HaveOccurred())

		oneShotMigration := NewOneShotMigrationService(fakeClient, prefix)

		migrateCount := 0

		for i := 0; i < 10; i++ {
			vm := &v1alpha2.VirtualMachine{}
			err := fakeClient.Get(context.Background(), client.ObjectKey{Namespace: vmNamespace, Name: vmName}, vm)
			Expect(err).ToNot(HaveOccurred())

			migrate, err := oneShotMigration.OnceMigrate(testutil.ContextBackgroundWithNoOpLogger(), vm, "key", "value")
			Expect(err).ToNot(HaveOccurred())
			if migrate {
				migrateCount++
			}
		}
		Expect(migrateCount).To(Equal(1))

		vmops := v1alpha2.VirtualMachineOperationList{}
		err = fakeClient.List(context.Background(), &vmops)
		Expect(err).ToNot(HaveOccurred())

		Expect(vmops.Items).To(HaveLen(1))
		vmop := vmops.Items[0]

		Expect(vmop.Name).To(HavePrefix(prefix))
	})
})
