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

package handler

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestVmopHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VMOP Restore/Clone handlers Suite")
}

func setupEnvironment(vmop *v1alpha2.VirtualMachineOperation, objs ...client.Object) (client.WithWatch, *reconciler.Resource[*v1alpha2.VirtualMachineOperation, v1alpha2.VirtualMachineOperationStatus]) {
	GinkgoHelper()
	Expect(vmop).ToNot(BeNil())

	allObjects := make([]client.Object, len(objs)+1)
	allObjects[0] = vmop
	allObjects = append(allObjects, objs...)

	fakeClient, err := testutil.NewFakeClientWithObjects(allObjects...)
	Expect(err).NotTo(HaveOccurred())

	srv := reconciler.NewResource(client.ObjectKeyFromObject(vmop), fakeClient,
		func() *v1alpha2.VirtualMachineOperation {
			return &v1alpha2.VirtualMachineOperation{}
		},
		func(obj *v1alpha2.VirtualMachineOperation) v1alpha2.VirtualMachineOperationStatus {
			return obj.Status
		})
	err = srv.Fetch(context.Background())
	Expect(err).NotTo(HaveOccurred())

	return fakeClient, srv
}
