/*
Copyright 2024 Flant JSC

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

package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/executor"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const (
	CurlPod           = "curl-helper"
	Timeout           = 90 * time.Second
	Interval          = 5 * time.Second
	externalHost      = "https://flant.com"
	nginxActiveStatus = "active"
)

var httpStatusOk = fmt.Sprintf("%v", http.StatusOK)

type PodEntrypoint struct {
	Command string
	Args    []string
}

func RunPod(podName, namespace, image string, entrypoint PodEntrypoint) *executor.CMDResult {
	GinkgoHelper()
	cmd := fmt.Sprintf("run %s --namespace %s --image=%s", podName, namespace, image)
	if entrypoint.Command != "" {
		cmd = fmt.Sprintf("%s --command %s", cmd, entrypoint.Command)
	}
	if entrypoint.Command != "" && len(entrypoint.Args) != 0 {
		rawArgs := strings.Join(entrypoint.Args, " ")
		cmd = fmt.Sprintf("%s -- %s", cmd, rawArgs)
	}
	return kubectl.RawCommand(cmd, ShortWaitDuration)
}

func GenerateServiceUrl(svc *corev1.Service, namespace string) string {
	service := fmt.Sprintf("%s.%s.svc:%d", svc.Name, namespace, svc.Spec.Ports[0].Port)
	return service
}

func GetResponseViaPodWithCurl(podName, namespace, host string) *executor.CMDResult {
	cmd := fmt.Sprintf("exec --namespace %s %s -- curl -o - %s", namespace, podName, host)
	return kubectl.RawCommand(cmd, ShortWaitDuration)
}

func CheckExternalConnection(host, httpCode string, vms ...string) {
	GinkgoHelper()
	for _, vm := range vms {
		By(fmt.Sprintf("Response code from %q should be %q for %q", host, httpCode, vm))
		cmd := fmt.Sprintf("curl -o /dev/null -s -w \"%%{http_code}\\n\" %s", host)
		CheckResultSshCommand(vm, cmd, httpCode)
	}
}

func CheckResultSshCommand(vmName, cmd, equal string) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		res := d8Virtualization.SshCommand(vmName, cmd, d8.SshOptions{
			Namespace:   conf.Namespace,
			Username:    conf.TestData.SshUser,
			IdenityFile: conf.TestData.Sshkey,
		})
		g.Expect(res.Error()).NotTo(HaveOccurred(), "result check failed for %s/%s.\n%s\n", conf.Namespace, vmName, res.StdErr())
		g.Expect(strings.TrimSpace(res.StdOut())).To(Equal(equal))
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
}

var _ = Describe("VM connectivity", Ordered, ContinueOnFailure, func() {
	var (
		testCaseLabel = map[string]string{"testcase": "vm-connectivity"}
		aObjName      = fmt.Sprintf("%s-vm-connectivity-a", namePrefix)
		bObjName      = fmt.Sprintf("%s-vm-connectivity-b", namePrefix)
		vmA, vmB      virtv2.VirtualMachine
		svcA, svcB    corev1.Service
		err           error
	)

	Context("When resources are applied:", func() {
		It("result should be succeeded", func() {
			res := kubectl.Kustomize(conf.TestData.Connectivity, kc.KustomizeOptions{})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual images are applied:", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be in %s phases", PhaseReady))
			WaitPhase(kc.ResourceVI, PhaseReady, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual disks are applied:", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be in %s phase", PhaseReady))
			WaitPhase(kc.ResourceVD, PhaseReady, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context("When virtual machines are applied:", func() {
		It("checks VMs phases", func() {
			By(fmt.Sprintf("VMs should be in %s phase", PhaseRunning))
			WaitPhase(kc.ResourceVM, PhaseRunning, kc.GetOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Output:    "jsonpath='{.items[*].metadata.name}'",
			})
		})
	})

	Context(fmt.Sprintf("When run %s:", CurlPod), func() {
		It(fmt.Sprintf("status should be in %s phase", PhaseRunning), func() {
			jsonPath := "jsonpath={.status.phase}"
			waitFor := fmt.Sprintf("%s=%s", jsonPath, PhaseRunning)
			res := RunPod(CurlPod, conf.Namespace, conf.HelperImages.CurlImage, PodEntrypoint{
				Command: "sleep",
				Args:    []string{"10000"},
			})
			Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
			WaitResource(kc.ResourcePod, CurlPod, waitFor, ShortWaitDuration)
		})
	})

	Context(fmt.Sprintf("When virtual machines in %s phase:", PhaseRunning), func() {
		It("gets VMs and SVCs objects", func() {
			vmA = virtv2.VirtualMachine{}
			err = GetObject(kc.ResourceVM, aObjName, &vmA, kc.GetOptions{
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred(), err)
			vmB = virtv2.VirtualMachine{}
			err = GetObject(kc.ResourceVM, bObjName, &vmB, kc.GetOptions{
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred(), err)

			svcA = corev1.Service{}
			err = GetObject(kc.ResourceService, aObjName, &svcA, kc.GetOptions{
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred(), err)
			svcB = corev1.Service{}
			err = GetObject(kc.ResourceService, bObjName, &svcB, kc.GetOptions{
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred(), err)
		})

		It("check ssh connection via `d8 v` to VMs", func() {
			cmd := "hostname"
			for _, vmName := range []string{vmA.Name, vmB.Name} {
				By(fmt.Sprintf("VirtualMachine %q", vmName))
				CheckResultSshCommand(vmName, cmd, vmName)
			}
		})

		It("checks VMs connection to external network", func() {
			CheckExternalConnection(externalHost, httpStatusOk, vmA.Name, vmB.Name)
		})

		It("check nginx status via `d8 v` on VMs", func() {
			cmd := "systemctl is-active nginx.service"
			for _, vmName := range []string{vmA.Name, vmB.Name} {
				By(fmt.Sprintf("VirtualMachine %q", vmName))
				CheckResultSshCommand(vmName, cmd, nginxActiveStatus)
			}
		})

		It(fmt.Sprintf("gets page from service %s", aObjName), func() {
			service := GenerateServiceUrl(&svcA, conf.Namespace)
			Eventually(func(g Gomega) {
				res := GetResponseViaPodWithCurl(CurlPod, conf.Namespace, service)
				g.Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
				g.Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vmA.Name))
			}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
		})

		It(fmt.Sprintf("gets page from service %s", bObjName), func() {
			service := GenerateServiceUrl(&svcB, conf.Namespace)
			Eventually(func(g Gomega) {
				res := GetResponseViaPodWithCurl(CurlPod, conf.Namespace, service)
				g.Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
				g.Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vmB.Name))
			}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
		})

		It(fmt.Sprintf("changes selector in service %s with selector from service %s", aObjName, bObjName), func() {
			PatchResource(kc.ResourceService, svcA.Name, &kc.JsonPatch{
				Op:    "replace",
				Path:  "/spec/selector/service",
				Value: svcB.Spec.Selector["service"],
			})
		})

		It(fmt.Sprintf("checks selector in service %s", aObjName), func() {
			By(fmt.Sprintf("Selector should be %q", svcB.Spec.Selector["service"]))
			label := svcB.Spec.Selector["service"]
			output := "jsonpath={.spec.selector.service}"
			CheckField(kc.ResourceService, svcA.Name, output, label)
		})

		It(fmt.Sprintf("gets page from service %s", aObjName), func() {
			By(fmt.Sprintf("Response should be from virtual machine %q", vmB.Name))
			service := GenerateServiceUrl(&svcA, conf.Namespace)
			Eventually(func(g Gomega) {
				res := GetResponseViaPodWithCurl(CurlPod, conf.Namespace, service)
				g.Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
				g.Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vmB.Name))
			}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
		})
	})
})
