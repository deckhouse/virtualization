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

package release

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

// createOrReuse creates objs like CreateWithDeferredDeletion, but keeps an
// object that already exists: a rerun of the pre-upgrade job lands in the
// Project of the failed attempt (see releaseNamespaceEnv) and picks up where it
// stopped. It returns the names of the objects that already existed.
func createOrReuse(ctx context.Context, f *framework.Framework, objs ...crclient.Object) map[string]bool {
	GinkgoHelper()

	reused := make(map[string]bool)
	for _, obj := range objs {
		err := f.CreateWithDeferredDeletion(ctx, obj)
		if k8serrors.IsAlreadyExists(err) {
			GinkgoWriter.Printf("Reusing %T %s/%s from an earlier attempt\n", obj, obj.GetNamespace(), obj.GetName())
			f.DeferDelete(obj)
			reused[obj.GetName()] = true
			continue
		}
		Expect(err).NotTo(HaveOccurred())
	}
	return reused
}

func (t *currentReleaseSmokeTest) diskObjects() []crclient.Object {
	objects := make([]crclient.Object, 0, len(t.vms)+len(t.dataDisks))
	for _, vmScenario := range t.vms {
		objects = append(objects, vmScenario.rootDisk)
	}
	for _, diskScenario := range t.dataDisks {
		objects = append(objects, diskScenario.disk)
	}
	return objects
}

func (t *currentReleaseSmokeTest) vmObjects() []crclient.Object {
	objects := make([]crclient.Object, 0, len(t.vms))
	for _, vmScenario := range t.vms {
		objects = append(objects, vmScenario.vm)
	}
	return objects
}

func (t *currentReleaseSmokeTest) attachmentObjects() []crclient.Object {
	objects := make([]crclient.Object, 0, len(t.attachments))
	for _, attachmentScenario := range t.attachments {
		objects = append(objects, attachmentScenario.attachment)
	}
	return objects
}

func (t *currentReleaseSmokeTest) manualStartVMs() []*vmScenario {
	manualVMs := make([]*vmScenario, 0)
	for _, vmScenario := range t.vms {
		if vmScenario.runPolicy == v1alpha2.ManualPolicy {
			manualVMs = append(manualVMs, vmScenario)
		}
	}
	return manualVMs
}
