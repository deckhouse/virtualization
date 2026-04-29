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

var _ = Describe("HotplugCPU", Label(precheck.NoPrecheck), func() {
	var (
		f *framework.Framework
		t *cpuHotplugTest
	)

	BeforeEach(func() {
		Skip("Hotplug CPU requires enabled feature gates in ModuleConfig. Skip until prechecks are implemented.")
		f = framework.NewFramework("cpu-hotplug")
		DeferCleanup(f.After)
		f.Before()
		t = newCPUHotplugTest(f)
	})

	DescribeTable("should apply cpu core changes without restart", func(initialCores, changedCores int) {
		By("Environment preparation")
		vmName := fmt.Sprintf("vm-%d-%d", initialCores, changedCores)
		t.generateResources(vmName, initialCores)
		err := f.CreateWithDeferredDeletion(context.Background(), t.VM, t.VD)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for VM agent to be ready")
		util.UntilSSHReady(f, t.VM, framework.MiddleTimeout)

		By("Checking initial CPU configuration")
		err = f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
		Expect(err).NotTo(HaveOccurred())
		Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(initialCores))

		guestCPUCount, err := t.getGuestCPUCount()
		Expect(err).NotTo(HaveOccurred())
		Expect(guestCPUCount).To(Equal(initialCores))

		By("Applying CPU core changes")
		patch, err := json.Marshal([]map[string]interface{}{{
			"op":    "replace",
			"path":  "/spec/cpu/cores",
			"value": changedCores,
		}})
		Expect(err).NotTo(HaveOccurred())
		err = f.GenericClient().Patch(context.Background(), t.VM, crclient.RawPatch(types.JSONPatchType, patch))
		Expect(err).NotTo(HaveOccurred())

		By("Waiting until CPU configuration is applied without restart")
		util.UntilVMMigrationSucceeded(crclient.ObjectKeyFromObject(t.VM), framework.MaxTimeout)
		util.UntilSSHReady(f, t.VM, framework.MiddleTimeout)

		By("Checking changed CPU configuration")
		err = f.Clients.GenericClient().Get(context.Background(), crclient.ObjectKeyFromObject(t.VM), t.VM)
		Expect(err).NotTo(HaveOccurred())
		Expect(t.VM.Status.Resources.CPU.Cores).To(Equal(changedCores))

		guestCPUCount, err = t.getGuestCPUCount()
		Expect(err).NotTo(HaveOccurred())
		Expect(guestCPUCount).To(Equal(changedCores))
	},
		Entry("one socket topology, change cores from 1 to 3", 1, 3),
		Entry("one socket topology, change cores to maximum 16", 4, 16),
	)
})

type cpuHotplugTest struct {
	Framework *framework.Framework

	VM *v1alpha2.VirtualMachine
	VD *v1alpha2.VirtualDisk
}

func newCPUHotplugTest(f *framework.Framework) *cpuHotplugTest {
	return &cpuHotplugTest{Framework: f}
}

func (t *cpuHotplugTest) generateResources(vmName string, cores int) {
	vdName := fmt.Sprintf("vd-%s-root", vmName)
	t.VD = object.NewVDFromCVI(vdName, t.Framework.Namespace().Name, object.PrecreatedCVIAlpineBIOS,
		vdbuilder.WithSize(ptr.To(resource.MustParse("350Mi"))),
	)

	t.VM = vmbuilder.New(
		vmbuilder.WithName(vmName),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithCPU(cores, ptr.To("10%")),
		vmbuilder.WithMemory(*resource.NewQuantity(object.Mi256, resource.BinarySI)),
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

func (t *cpuHotplugTest) getGuestCPUCount() (int, error) {
	cmdOut, err := t.Framework.SSHCommand(t.VM.Name, t.VM.Namespace, "nproc")
	if err != nil {
		return 0, err
	}

	var cpuCount int
	_, err = fmt.Sscanf(strings.TrimSpace(cmdOut), "%d", &cpuCount)
	if err != nil {
		return 0, fmt.Errorf("parse guest cpu count from %q: %w", cmdOut, err)
	}

	return cpuCount, nil
}
