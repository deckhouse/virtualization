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

package vm

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/executor"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/network"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	podobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/pod"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

var _ = Describe("VirtualMachineConnectivity", Label(label.SIGCompute, precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		t   *VMConnectivityTest
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("vm-connectivity")
		DeferCleanup(f.After)
		f.Before()
		t = NewVMConnectivityTest(f)
	})

	It("checks VM network connectivity", func() {
		By("Environment preparation", func() {
			t.GenerateEnvironmentResources()
			err := f.CreateWithDeferredDeletion(ctx, t.VDa, t.VDb, t.VMa, t.VMb, t.ServiceA, t.ServiceB, t.CurlPod)
			Expect(err).NotTo(HaveOccurred())

			vmaObs := vmobs.StartObserver(ctx, f, t.VMa)
			vmaObs.Never(vmobs.BeFailed())
			vmbObs := vmobs.StartObserver(ctx, f, t.VMb)
			vmbObs.Never(vmobs.BeFailed())
			curlPodObs := podobs.StartObserver(ctx, f, t.CurlPod)

			err = vmaObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = vmbObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = vmaObs.WaitFor(vmobs.BeAgentReady(), framework.MiddleTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = vmbObs.WaitFor(vmobs.BeAgentReady(), framework.MiddleTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = curlPodObs.WaitFor(podobs.BeRunning(), framework.ShortTimeout)
			Expect(err).NotTo(HaveOccurred())

			t.PublishGuestPages(framework.LongTimeout)
		})

		// There is a known issue with the Cilium agent check.
		By("Check Cilium agents are properly configured for the VMs", func() {
			network.EnsureCiliumAgents(ctx, f.Kubectl(), t.VMa.Name, f.Namespace().Name)
			network.EnsureCiliumAgents(ctx, f.Kubectl(), t.VMb.Name, f.Namespace().Name)
		})

		By("Check VMs can reach external network", func() {
			// The custom guest has no curl (BusyBox userspace), so outbound
			// connectivity is probed with ping as root.
			for _, vmName := range []string{t.VMa.Name, t.VMb.Name} {
				reachableHost, err := f.SSHCommand(vmName, f.Namespace().Name, guestPingExternalCommand, framework.WithSSHUser("root"))
				Expect(err).NotTo(HaveOccurred(), "VM %s should have outbound connectivity", vmName)
				Expect(reachableHost).NotTo(BeEmpty())
			}
		})

		By("Check httpd serves the guest page on VMs", func() {
			cmd := "wget -qO- http://127.0.0.1/"

			cmdStdOutA, err := f.SSHCommand(t.VMa.Name, t.VMa.Namespace, cmd, framework.WithSSHUser("root"))
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(cmdStdOutA)).To(ContainSubstring(t.VMa.Name))

			cmdStdOutB, err := f.SSHCommand(t.VMb.Name, t.VMb.Namespace, cmd, framework.WithSSHUser("root"))
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(cmdStdOutB)).To(ContainSubstring(t.VMb.Name))
		})

		By("Check response from service on VMs", func() {
			resA := t.GetResponseViaPodWithCurl(t.CurlPod.Name, t.CurlPod.Namespace, t.ServiceA)
			Expect(resA.Error()).NotTo(HaveOccurred())
			Expect(resA.StdOut()).To(ContainSubstring(t.VMa.Name))

			resB := t.GetResponseViaPodWithCurl(t.CurlPod.Name, t.CurlPod.Namespace, t.ServiceB)
			Expect(resB.Error()).NotTo(HaveOccurred())
			Expect(resB.StdOut()).To(ContainSubstring(t.VMb.Name))
		})

		By("Replace selector in service A with selector from service B", func() {
			t.ServiceA.Spec.Selector["service"] = t.SelectorB
			err := f.Clients.GenericClient().Update(ctx, t.ServiceA)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Check response from service A on VM B", func() {
			res := t.GetResponseViaPodWithCurl(t.CurlPod.Name, t.CurlPod.Namespace, t.ServiceA)
			Expect(res.Error()).NotTo(HaveOccurred())
			Expect(res.StdOut()).To(ContainSubstring(t.VMb.Name))
		})

		By("Change selector in service A back to selector from service A", func() {
			t.ServiceA.Spec.Selector["service"] = t.SelectorA
			err := f.Clients.GenericClient().Update(ctx, t.ServiceA)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Check response from service A on VM A", func() {
			res := t.GetResponseViaPodWithCurl(t.CurlPod.Name, t.CurlPod.Namespace, t.ServiceA)
			Expect(res.Error()).NotTo(HaveOccurred())
			Expect(res.StdOut()).To(ContainSubstring(t.VMa.Name))
		})
	})
})

type VMConnectivityTest struct {
	Framework *framework.Framework

	VDa *v1alpha2.VirtualDisk
	VDb *v1alpha2.VirtualDisk
	VMa *v1alpha2.VirtualMachine
	VMb *v1alpha2.VirtualMachine

	ServiceA *corev1.Service
	ServiceB *corev1.Service

	CurlPod *corev1.Pod

	SelectorA string
	SelectorB string
}

func NewVMConnectivityTest(f *framework.Framework) *VMConnectivityTest {
	return &VMConnectivityTest{
		Framework: f,
		SelectorA: "vm-a",
		SelectorB: "vm-b",
	}
}

func (t *VMConnectivityTest) GenerateEnvironmentResources() {
	// The custom image bakes in a BusyBox httpd serving /var/www on :80
	// from boot; the test publishes each guest's page (its VM name) over SSH,
	// so no cloud-init/nginx is needed.
	t.VDa = object.NewVDFromCVI("vd-a", t.Framework.Namespace().Name, object.PrecreatedCVICustomBIOS, vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))))
	t.VDb = object.NewVDFromCVI("vd-b", t.Framework.Namespace().Name, object.PrecreatedCVICustomBIOS, vdbuilder.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))))

	t.VMa = vmbuilder.New(
		vmbuilder.WithName("vm-a"),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithLabel("service", "vm-a"),
		vmbuilder.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
		vmbuilder.WithMemory(resource.MustParse(object.CustomImageVMMemory)),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: t.VDa.Name,
			},
		),
	)

	t.VMb = vmbuilder.New(
		vmbuilder.WithName("vm-b"),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithLabel("service", "vm-b"),
		vmbuilder.WithCPU(1, ptr.To(object.CustomImageVMCoreFraction)),
		vmbuilder.WithMemory(resource.MustParse(object.CustomImageVMMemory)),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithBlockDeviceRefs(
			v1alpha2.BlockDeviceSpecRef{
				Kind: v1alpha2.DiskDevice,
				Name: t.VDb.Name,
			},
		),
	)

	t.ServiceA = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vm-a-service",
			Namespace: t.Framework.Namespace().Name,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"service": t.SelectorA},
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 80,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 80,
					},
				},
			},
		},
	}

	t.ServiceB = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vm-b-service",
			Namespace: t.Framework.Namespace().Name,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"service": t.SelectorB},
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 80,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 80,
					},
				},
			},
		},
	}

	t.CurlPod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "curl-helper",
			Namespace: t.Framework.Namespace().Name,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "curl",
					Image:   framework.GetConfig().HelperImages.CurlImage,
					Command: []string{"sleep"},
					Args:    []string{"10000"},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						RunAsNonRoot:             ptr.To(true),
						RunAsUser:                ptr.To(int64(1000)),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				},
			},
		},
	}
}

// PublishGuestPages writes each guest page (the VM name) into the BusyBox
// httpd webroot, so a curl through the Service identifies which VM answered.
// httpd itself is baked into the custom image and serves /var/www
// from boot.
//
// The only asynchronous part is the guest finishing its boot, so wait for SSH
// readiness once and then run the command a single time: it is deterministic
// as soon as the guest accepts SSH.
func (t *VMConnectivityTest) PublishGuestPages(timeout time.Duration) {
	GinkgoHelper()

	for _, vm := range []*v1alpha2.VirtualMachine{t.VMa, t.VMb} {
		eventually.SSHReadyAsRoot(t.Framework, vm, timeout)

		cmd := fmt.Sprintf("mkdir -p /var/www && echo %s > /var/www/index.html && cat /var/www/index.html", vm.Name)
		out, err := t.Framework.SSHCommand(vm.Name, vm.Namespace, cmd, framework.WithSSHUser("root"))
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring(vm.Name))
	}
}

func (t *VMConnectivityTest) GetResponseViaPodWithCurl(podName, namespace string, service *corev1.Service) *executor.CMDResult {
	url := t.generateServiceURL(service)
	cmd := fmt.Sprintf("exec --namespace %s %s -- curl -o - %s", namespace, podName, url)
	return t.Framework.Kubectl().RawCommand(cmd, framework.ShortTimeout)
}

func (t *VMConnectivityTest) generateServiceURL(svc *corev1.Service) string {
	service := fmt.Sprintf("%s.%s.svc:%d", svc.Name, svc.Namespace, svc.Spec.Ports[0].Port)
	return service
}
