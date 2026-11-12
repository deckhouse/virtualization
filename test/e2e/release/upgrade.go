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
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	dv1alpha2 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	projobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/project"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

// The suite runs twice around a module upgrade performed by someone else:
// pre-upgrade creates and checks the virtual machines and leaves them in the
// cluster, post-upgrade finds them by name and checks the upgrade did not
// disturb them.
const (
	phaseEnv         = "UPGRADE_PHASE"
	PhasePreUpgrade  = "pre-upgrade"
	PhasePostUpgrade = "post-upgrade"

	// "false" for an upgrade that ships the same virt-launcher and so never
	// migrates a virtual machine: the checks that need a migration are skipped.
	migratesVMsEnv = "UPGRADE_MIGRATES_VMS"

	// The Project, and so the namespace, that holds the virtual machines between
	// the phases; the e2e prefix lets "task cleanup" remove it.
	projectName = framework.NamespaceBasePrefix + "-upgrade"
)

func GetPhase() string {
	GinkgoHelper()

	phase := os.Getenv(phaseEnv)
	Expect(phase).To(BeElementOf(PhasePreUpgrade, PhasePostUpgrade),
		"%s must be %q or %q", phaseEnv, PhasePreUpgrade, PhasePostUpgrade)
	return phase
}

func migratesVMs() bool {
	return os.Getenv(migratesVMsEnv) != "false"
}

// CreateScenarioResources runs on process 1 of SynchronizedBeforeSuite before
// the upgrade: the image, the Project and every disk, virtual machine and
// attachment of the scenario tables. The specs then only wait and check.
func CreateScenarioResources(ctx context.Context, f *framework.Framework) {
	GinkgoHelper()

	Expect(precheck.EnsurePrecreatedCVIs(ctx, f, object.FindPrecreatedClusterVirtualImage(object.PrecreatedCVICustomBIOS))).To(Succeed())
	recreateProject(ctx, f)

	storageClasses := framework.GetConfig().StorageClass
	Expect(storageClasses.DefaultStorageClass).NotTo(BeNil())
	Expect(storageClasses.LocalStorageClass).NotTo(BeNil())
	replicated := storageClasses.DefaultStorageClass.Name
	local := storageClasses.LocalStorageClass.Name

	By("Creating root and data disks")
	disks := make([]crclient.Object, 0, len(vmScenarios)+len(dataDiskScenarios))
	for _, s := range vmScenarios {
		disks = append(disks, s.newRootDisk(replicated))
	}
	dataDisks := make(map[string]*v1alpha2.VirtualDisk, len(dataDiskScenarios))
	for _, d := range dataDiskScenarios {
		storageClass := replicated
		if d.local {
			storageClass = local
		}
		dataDisks[d.name] = d.newDisk(storageClass)
		disks = append(disks, dataDisks[d.name])
	}
	createObjects(ctx, f, disks...)

	By("Creating virtual machines")
	vms := make([]crclient.Object, 0, len(vmScenarios))
	for _, s := range vmScenarios {
		vms = append(vms, s.newVM())
	}
	createObjects(ctx, f, vms...)

	By("Creating hotplug attachments")
	attachments := make([]crclient.Object, 0, len(dataDiskScenarios))
	for _, d := range dataDiskScenarios {
		attachments = append(attachments, d.newAttachment(dataDisks[d.name]))
	}
	createObjects(ctx, f, attachments...)
}

// ExpectProjectExists runs on process 1 of SynchronizedBeforeSuite after the
// upgrade: the Project with the virtual machines of the pre-upgrade phase must
// still be there.
func ExpectProjectExists(ctx context.Context, f *framework.Framework) {
	GinkgoHelper()

	project := &dv1alpha2.Project{}
	err := f.GenericClient().Get(ctx, crclient.ObjectKey{Name: projectName}, project)
	Expect(err).NotTo(HaveOccurred(),
		"project %q with the virtual machines of the pre-upgrade phase must exist", projectName)
	Expect(project.Status.State).To(Equal(dv1alpha2.ProjectStateDeployed),
		"project %q must be Deployed, got %q", projectName, project.Status.State)
}

// recreateProject gives the suite a fresh Project (and so a fresh namespace)
// named projectName, deleting the one left from a previous run.
func recreateProject(ctx context.Context, f *framework.Framework) {
	GinkgoHelper()

	project := &dv1alpha2.Project{ObjectMeta: metav1.ObjectMeta{Name: projectName}}
	err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(project), project)
	switch {
	case err == nil:
		By(fmt.Sprintf("Project %q is left from a previous run, recreating it", projectName))
		Expect(f.Delete(ctx, project)).To(Succeed())
	case !k8serrors.IsNotFound(err):
		Expect(err).NotTo(HaveOccurred())
	}

	project = object.NewNonIsolatedProject("upgrade", framework.NamespaceBasePrefix)
	project.GenerateName = ""
	project.Name = projectName
	project.Labels = map[string]string{framework.E2ELabel: "true"}
	projectObs := projobs.StartObserver(ctx, f, project.Name)
	Expect(f.GenericClient().Create(ctx, project)).To(Succeed())
	Expect(projectObs.WaitFor(projobs.BeDeployed(), framework.LongTimeout)).To(Succeed(),
		"project %q must be Deployed", projectName)
	By(fmt.Sprintf("Project %q has been created", projectName))
}

func createObjects(ctx context.Context, f *framework.Framework, objs ...crclient.Object) {
	GinkgoHelper()

	for _, obj := range objs {
		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[framework.E2ELabel] = "true"
		obj.SetLabels(labels)

		Expect(f.GenericClient().Create(ctx, obj)).To(Succeed(), "create %s/%s", obj.GetNamespace(), obj.GetName())
	}
}

func getVirtualMachine(ctx context.Context, f *framework.Framework, vm *v1alpha2.VirtualMachine) *v1alpha2.VirtualMachine {
	GinkgoHelper()

	current := &v1alpha2.VirtualMachine{}
	err := f.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vm), current)
	Expect(err).NotTo(HaveOccurred())
	return current
}
