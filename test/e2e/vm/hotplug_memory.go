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

package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("HotplugMemory", Label(precheck.NoPrecheck), func() {
	var (
		f *framework.Framework
		t *memoryHotplugTest
	)

	BeforeEach(func() {
		Skip("Hotplug memory requires enabling feature gate 'HotplugMemoryWithLiveMigration' in ModuleConfig. Skip until prechecks are implemented.")
		f = framework.NewFramework("memory-hotplug")
		DeferCleanup(f.After)
		f.Before()
		t = newMemoryHotplugTest(f)
	})

	DescribeTable("should apply memory changes with live migration", func(initialMemory, changedMemory string) {
		initialQuantity := resource.MustParse(initialMemory)
		changedQuantity := resource.MustParse(changedMemory)

		By("Environment preparation")
		vmName := strings.ToLower(fmt.Sprintf("vm-%s-%s", initialMemory, changedMemory))
		t.generateResources(vmName, initialMemory)
		err := f.CreateWithDeferredDeletion(context.Background(), t.VM, t.VD)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VM agent to be ready")
		util.UntilSSHReady(f, t.VM, framework.MiddleTimeout)

		By("Checking initial memory size")
		err = f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
		Expect(err).NotTo(HaveOccurred())
		Expect(t.VM.Status.Resources.Memory.Size).To(Equal(initialQuantity))

		guestMemorySize, err := t.getGuestMemorySize()
		Expect(err).NotTo(HaveOccurred())
		Expect(guestMemorySize).To(Equal(int(initialQuantity.Value())))

		By("Applying memory size changes")
		patch, err := json.Marshal([]map[string]interface{}{{
			"op":    "replace",
			"path":  "/spec/memory/size",
			"value": changedMemory,
		}})
		Expect(err).NotTo(HaveOccurred())
		err = f.GenericClient().Patch(context.Background(), t.VM, crclient.RawPatch(types.JSONPatchType, patch))
		Expect(err).NotTo(HaveOccurred())

		By("Waiting until memory size is applied without restart")
		util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(t.VM), framework.MaxTimeout)
		util.UntilSSHReady(f, t.VM, framework.MiddleTimeout)

		By("Checking changed memory size")
		err = f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
		Expect(err).NotTo(HaveOccurred())
		Expect(t.VM.Status.Resources.Memory.Size).To(Equal(changedQuantity))

		guestMemorySize, err = t.getGuestMemorySize()
		Expect(err).NotTo(HaveOccurred())
		Expect(guestMemorySize).To(Equal(int(changedQuantity.Value())))
	},
		Entry("change memory from 1Gi to 2Gi", "1Gi", "2Gi"),
	)
})

type memoryHotplugTest struct {
	Framework *framework.Framework

	VM *v1alpha2.VirtualMachine
	VD *v1alpha2.VirtualDisk
}

func newMemoryHotplugTest(f *framework.Framework) *memoryHotplugTest {
	return &memoryHotplugTest{Framework: f}
}

func (t *memoryHotplugTest) generateResources(vmName, memSize string) {
	memSizeQuantity := resource.MustParse(memSize)

	vdName := fmt.Sprintf("vd-%s-root", vmName)
	t.VD = object.NewVDFromCVI(vdName, t.Framework.Namespace().Name, object.PrecreatedCVIAlpineBIOS,
		vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))),
	)

	t.VM = vmbuilder.New(
		vmbuilder.WithName(vmName),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithCPU(2, ptr.To("10%")),
		vmbuilder.WithMemory(memSizeQuantity),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithProvisioningUserData(object.AlpineCloudInit),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: t.VD.Name,
			},
		),
		vmbuilder.WithRestartApprovalMode(v1alpha2.Automatic),
	)
}

var totalOnlineMemRe = regexp.MustCompile(`^Total online memory:\s+(\d+)$`)

func (t *memoryHotplugTest) getGuestMemorySize() (int, error) {
	cmdOut, err := t.Framework.SSHCommand(t.VM.Name, t.VM.Namespace, "lsmem -b --summary=only")
	if err != nil {
		return 0, err
	}

	lines := strings.Split(cmdOut, "\n")

	for _, line := range lines {
		matches := totalOnlineMemRe.FindStringSubmatch(line)
		if len(matches) >= 2 {
			return strconv.Atoi(matches[1])
		}
	}

	return 0, fmt.Errorf("failed to find total online memory in lsmem output: %v", cmdOut)
}
