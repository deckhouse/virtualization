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
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/executor"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/network"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

var _ = Describe("VirtualMachineConnectivity", func() {
	var (
		f *framework.Framework
		t *VMConnectivityTest
	)

	BeforeEach(func() {
		f = framework.NewFramework("vm-connectivity")
		DeferCleanup(f.After)
		f.Before()
		t = NewVMConnectivityTest(f)
	})

	It("checks VM network connectivity", func() {
		By("Environment preparation", func() {
			t.GenerateEnvironmentResources()
			err := f.CreateWithDeferredDeletion(context.Background(), t.VDa, t.VDb, t.VMa, t.VMb, t.ServiceA, t.ServiceB, t.CurlPod)
			Expect(err).NotTo(HaveOccurred())

			util.UntilObjectPhase(string(v1alpha2.MachineRunning), framework.LongTimeout, t.VMa, t.VMb)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(t.VMa), framework.MiddleTimeout)
			util.UntilVMAgentReady(crclient.ObjectKeyFromObject(t.VMb), framework.MiddleTimeout)
			util.UntilObjectPhase(string(corev1.PodRunning), framework.ShortTimeout, t.CurlPod)

			t.CheckCloudInitCompleted(framework.LongTimeout)
		})

		By("Check Cilium agents are properly configured for the VMs", func() {
			err := network.CheckCiliumAgents(context.Background(), f.Clients.Kubectl(), t.VMa.Name, f.Namespace().Name)
			Expect(err).NotTo(HaveOccurred(), "Cilium agents check should succeed for VM %s", t.VMa.Name)
			err = network.CheckCiliumAgents(context.Background(), f.Clients.Kubectl(), t.VMb.Name, f.Namespace().Name)
			Expect(err).NotTo(HaveOccurred(), "Cilium agents check should succeed for VM %s", t.VMb.Name)
		})

		By("Check VMs can reach external network", func() {
			network.CheckExternalConnectivity(f, t.VMa.Name, network.ExternalHost, network.HTTPStatusOk)
			network.CheckExternalConnectivity(f, t.VMb.Name, network.ExternalHost, network.HTTPStatusOk)
		})

		By("Check nginx status on VMs", func() {
			cmd := "systemctl is-active nginx.service"
			expectedOut := "active"

			cmdStdOutA, err := f.SSHCommand(t.VMa.Name, t.VMa.Namespace, cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(cmdStdOutA)).To(Equal(expectedOut))

			cmdStdOutB, err := f.SSHCommand(t.VMb.Name, t.VMb.Namespace, cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(cmdStdOutB)).To(Equal(expectedOut))
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
			err := f.Clients.GenericClient().Update(context.Background(), t.ServiceA)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Check response from service A on VM B", func() {
			res := t.GetResponseViaPodWithCurl(t.CurlPod.Name, t.CurlPod.Namespace, t.ServiceA)
			Expect(res.Error()).NotTo(HaveOccurred())
			Expect(res.StdOut()).To(ContainSubstring(t.VMb.Name))
		})

		By("Change selector in service A back to selector from service A", func() {
			t.ServiceA.Spec.Selector["service"] = t.SelectorA
			err := f.Clients.GenericClient().Update(context.Background(), t.ServiceA)
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
	t.VDa = vdbuilder.New(
		vdbuilder.WithName("vd-a"),
		vdbuilder.WithNamespace(t.Framework.Namespace().Name),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: object.ImageURLUbuntu,
		}),
	)

	t.VDb = vdbuilder.New(
		vdbuilder.WithName("vd-b"),
		vdbuilder.WithNamespace(t.Framework.Namespace().Name),
		vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
			URL: object.ImageURLUbuntu,
		}),
	)

	t.VMa = vmbuilder.New(
		vmbuilder.WithName("vm-a"),
		vmbuilder.WithNamespace(t.Framework.Namespace().Name),
		vmbuilder.WithLabel("service", "vm-a"),
		vmbuilder.WithCPU(1, ptr.To("50%")),
		vmbuilder.WithMemory(resource.MustParse("1Gi")),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithProvisioningUserData(t.getCloudInit()),
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
		vmbuilder.WithCPU(1, ptr.To("50%")),
		vmbuilder.WithMemory(resource.MustParse("1Gi")),
		vmbuilder.WithLiveMigrationPolicy(v1alpha2.AlwaysSafeMigrationPolicy),
		vmbuilder.WithVirtualMachineClass(object.DefaultVMClass),
		vmbuilder.WithProvisioningUserData(t.getCloudInit()),
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
				},
			},
		},
	}
}

func (t *VMConnectivityTest) CheckCloudInitCompleted(timeout time.Duration) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		cmd := "cat /var/www/html/index.html"
		cmdStdOutA, err := t.Framework.SSHCommand(t.VMa.Name, t.VMa.Namespace, cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cmdStdOutA).To(ContainSubstring(t.VMa.Name))

		cmdStdOutB, err := t.Framework.SSHCommand(t.VMb.Name, t.VMb.Namespace, cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cmdStdOutB).To(ContainSubstring(t.VMb.Name))
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
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

func (t *VMConnectivityTest) getCloudInit() string {
	return `#cloud-config
      package_update: true
      packages:
      - qemu-guest-agent
      - nginx
      write_files:
        - path: /usr/scripts/genpage_script.sh
          permissions: "0755"
          content: |
            #!/bin/bash
            rm -f /var/www/html/index*

            cat > /var/www/html/index.html<<EOF
            <!DOCTYPE html>
            <html>
            <head>
            <title>Welcome to $(hostname)<title>
            </head>
            <body>
            <h1>Welcome to nginx on server $(hostname)!</h1>
            </body>
            </html>
            EOF
      runcmd:
      - [ /usr/scripts/genpage_script.sh ]
      - [ systemctl, daemon-reload ]
      - [ systemctl, enable, --now, qemu-guest-agent.service ]
      - [ systemctl, enable, --now, nginx ]
      users:
      - name: cloud
        # passwd: cloud
        passwd: $6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.
        shell: /bin/bash
        sudo: ALL=(ALL) NOPASSWD:ALL
        lock_passwd: false
        ssh_authorized_keys:
        # testcases
        - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFxcXHmwaGnJ8scJaEN5RzklBPZpVSic4GdaAsKjQoeA your_email@example.com
`
}
