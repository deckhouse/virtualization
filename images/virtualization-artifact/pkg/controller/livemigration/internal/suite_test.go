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

package internal

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
)

func TestHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Live migration handlers suite")
}

func setupEnvironment(kvvmi *virtv1.VirtualMachineInstance, objs ...client.Object) client.WithWatch {
	GinkgoHelper()
	Expect(kvvmi).ToNot(BeNil(), "Should set KVVMI for setupEnvironment")
	allObjects := []client.Object{kvvmi}
	allObjects = append(allObjects, objs...)

	fakeClient, err := testutil.NewFakeClientWithObjects(allObjects...)
	Expect(err).NotTo(HaveOccurred())

	return fakeClient
}
