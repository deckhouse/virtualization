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

package legacy

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/d8"
	"github.com/deckhouse/virtualization/test/e2e/internal/executor"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	kc "github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
	"github.com/deckhouse/virtualization/test/e2e/internal/network"
)

const (
	CurlPod           = "curl-helper"
	externalHost      = "https://flant.com"
	nginxActiveStatus = "active"
)

var _ = Describe("VirtualMachineConnectivity", Ordered, func() {
	var (
		testCaseLabel = map[string]string{"testcase": "vm-connectivity"}
		aObjName      = fmt.Sprintf("%s-vm-connectivity-a", namePrefix)
		bObjName      = fmt.Sprintf("%s-vm-connectivity-b", namePrefix)
		vmA, vmB      v1alpha2.VirtualMachine
		svcA, svcB    corev1.Service
		ns            string

		selectorA string
		selectorB string
	)

	BeforeAll(func() {
		kustomization := fmt.Sprintf("%s/%s", conf.TestData.Connectivity, "kustomization.yaml")
		var err error
		ns, err = kustomize.GetNamespace(kustomization)
		Expect(err).NotTo(HaveOccurred(), "%w", err)

		CreateNamespace(ns)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestCaseDump(testCaseLabel, CurrentSpecReport().LeafNodeText, ns)
		}
	})

	Context("When resources are applied", func() {
		It("result should be succeeded", func() {
			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.Connectivity},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual images are applied", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVI, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual disks are applied", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be in %s phase", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied", func() {
		It("checks VMs phases", func() {
			By("Virtual machine agents should be ready")
			WaitVMAgentReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: ns,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context(fmt.Sprintf("When run %s", CurlPod), func() {
		It(fmt.Sprintf("status should be in %s phase", PhaseRunning), func() {
			jsonPath := "jsonpath={.status.phase}"
			waitFor := fmt.Sprintf("%s=%s", jsonPath, PhaseRunning)
			res := RunPod(CurlPod, ns, conf.HelperImages.CurlImage, PodEntrypoint{
				Command: "sleep",
				Args:    []string{"10000"},
			})
			Expect(res.Error()).NotTo(HaveOccurred())
			WaitResource(kc.ResourcePod, ns, CurlPod, waitFor, ShortWaitDuration)
		})
	})

	Context("When virtual machine agents are ready", func() {
		It("gets VMs and SVCs objects", func() {
			vmA = v1alpha2.VirtualMachine{}
			err := GetObject(kc.ResourceVM, aObjName, &vmA, kc.GetOptions{
				Namespace: ns,
			})
			Expect(err).NotTo(HaveOccurred())
			vmB = v1alpha2.VirtualMachine{}
			err = GetObject(kc.ResourceVM, bObjName, &vmB, kc.GetOptions{
				Namespace: ns,
			})
			Expect(err).NotTo(HaveOccurred())

			svcA = corev1.Service{}
			err = GetObject(kc.ResourceService, aObjName, &svcA, kc.GetOptions{
				Namespace: ns,
			})
			Expect(err).NotTo(HaveOccurred())
			svcB = corev1.Service{}
			err = GetObject(kc.ResourceService, bObjName, &svcB, kc.GetOptions{
				Namespace: ns,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("check ssh connection via `d8 v` to VMs", func() {
			cmd := "hostname"
			for _, vmName := range []string{vmA.Name, vmB.Name} {
				By(fmt.Sprintf("VirtualMachine %q", vmName))
				CheckResultSSHCommand(ns, vmName, cmd, vmName)
			}
		})

		It("checks VMs connection to external network", func() {
			CheckCiliumAgents(kubectl, ns, vmA.Name, vmB.Name)
			CheckExternalConnection(externalHost, httpStatusOk, ns, vmA.Name, vmB.Name)
		})

		It("check nginx status via `d8 v` on VMs", func() {
			cmd := "systemctl is-active nginx.service"
			for _, vmName := range []string{vmA.Name, vmB.Name} {
				By(fmt.Sprintf("VirtualMachine %q", vmName))
				CheckResultSSHCommand(ns, vmName, cmd, nginxActiveStatus)
			}
		})

		It(fmt.Sprintf("gets page from service %s", aObjName), func() {
			service := GenerateServiceURL(&svcA, ns)
			Eventually(func() (string, error) {
				res := GetResponseViaPodWithCurl(CurlPod, ns, service)
				if res.Error() != nil {
					return "", fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
				}
				return strings.TrimSpace(res.StdOut()), nil
			}).WithTimeout(Timeout).WithPolling(Interval).Should(ContainSubstring(vmA.Name))
		})

		It(fmt.Sprintf("gets page from service %s", bObjName), func() {
			service := GenerateServiceURL(&svcB, ns)
			Eventually(func() (string, error) {
				res := GetResponseViaPodWithCurl(CurlPod, ns, service)
				if res.Error() != nil {
					return "", fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
				}
				return strings.TrimSpace(res.StdOut()), nil
			}).WithTimeout(Timeout).WithPolling(Interval).Should(ContainSubstring(vmB.Name))
		})

		It(fmt.Sprintf("changes selector in service %s with selector from service %s", aObjName, bObjName), func() {
			selectorA = svcA.Spec.Selector["service"]
			selectorB = svcB.Spec.Selector["service"]

			PatchResource(kc.ResourceService, ns, svcA.Name, []*kc.JSONPatch{
				{
					Op:    "replace",
					Path:  "/spec/selector/service",
					Value: selectorB,
				},
			})
		})

		It(fmt.Sprintf("checks selector in service %s", aObjName), func() {
			By(fmt.Sprintf("Selector should be %q", selectorB))
			output := "jsonpath={.spec.selector.service}"
			CheckField(kc.ResourceService, ns, svcA.Name, output, selectorB)
		})

		It(fmt.Sprintf("gets page from service %s", aObjName), func() {
			By(fmt.Sprintf("Response should be from virtual machine %q", vmB.Name))
			service := GenerateServiceURL(&svcA, ns)
			Eventually(func() (string, error) {
				res := GetResponseViaPodWithCurl(CurlPod, ns, service)
				if res.Error() != nil {
					return "", fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
				}
				return strings.TrimSpace(res.StdOut()), nil
			}).WithTimeout(Timeout).WithPolling(Interval).Should(ContainSubstring(vmB.Name))
		})

		It(fmt.Sprintf("changes back selector in service %s", aObjName), func() {
			PatchResource(kc.ResourceService, ns, svcA.Name, []*kc.JSONPatch{
				{
					Op:    "replace",
					Path:  "/spec/selector/service",
					Value: selectorA,
				},
			})
		})

		It(fmt.Sprintf("checks selector in service %s", aObjName), func() {
			By(fmt.Sprintf("Selector should be %q", selectorA))
			output := "jsonpath={.spec.selector.service}"
			CheckField(kc.ResourceService, ns, svcA.Name, output, selectorA)
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			resourcesToDelete := ResourcesToDelete{
				AdditionalResources: []AdditionalResource{
					{
						Resource: kc.ResourcePod,
						Labels:   map[string]string{"name": CurlPod},
					},
				},
			}

			if config.IsCleanUpNeeded() {
				resourcesToDelete.KustomizationDir = conf.TestData.Connectivity
			}

			DeleteTestCaseResources(ns, resourcesToDelete)
		})
	})
})

var httpStatusOk = fmt.Sprintf("%v", http.StatusOK)

type PodEntrypoint struct {
	Command string
	Args    []string
}

func RunPod(podName, namespace, image string, entrypoint PodEntrypoint) *executor.CMDResult {
	GinkgoHelper()
	cmd := fmt.Sprintf("run %s --namespace %s --image=%s --labels='name=%s'", podName, namespace, image, podName)
	if entrypoint.Command != "" {
		cmd = fmt.Sprintf("%s --command %s", cmd, entrypoint.Command)
	}
	if entrypoint.Command != "" && len(entrypoint.Args) != 0 {
		rawArgs := strings.Join(entrypoint.Args, " ")
		cmd = fmt.Sprintf("%s -- %s", cmd, rawArgs)
	}
	return kubectl.RawCommand(cmd, ShortWaitDuration)
}

func GenerateServiceURL(svc *corev1.Service, namespace string) string {
	service := fmt.Sprintf("%s.%s.svc:%d", svc.Name, namespace, svc.Spec.Ports[0].Port)
	return service
}

func GetResponseViaPodWithCurl(podName, namespace, host string) *executor.CMDResult {
	cmd := fmt.Sprintf("exec --namespace %s %s -- curl -o - %s", namespace, podName, host)
	return kubectl.RawCommand(cmd, ShortWaitDuration)
}

func CheckCiliumAgents(kubectl kc.Kubectl, namespace string, vms ...string) {
	GinkgoHelper()
	for _, vm := range vms {
		By(fmt.Sprintf("Cilium agent should be OK's for VM: %s", vm))
		Eventually(func() error {
			return network.CheckCiliumAgents(context.Background(), kubectl, vm, namespace)
		}).
			WithTimeout(Timeout).
			WithPolling(Interval).
			Should(Succeed())
	}
}

func CheckExternalConnection(host, httpCode, vmNamespace string, vmNames ...string) {
	GinkgoHelper()
	for _, vmName := range vmNames {
		By(fmt.Sprintf("Response code from %q should be %q for %q", host, httpCode, vmName))
		cmd := fmt.Sprintf("curl -o /dev/null -s -w \"%%{http_code}\\n\" %s", host)
		CheckResultSSHCommand(vmNamespace, vmName, cmd, httpCode)
	}
}

func CheckResultSSHCommand(vmNamespace, vmName, cmd, equal string) {
	GinkgoHelper()
	Eventually(func() (string, error) {
		res := framework.GetClients().D8Virtualization().SSHCommand(vmName, cmd, d8.SSHOptions{
			Namespace:    vmNamespace,
			Username:     conf.TestData.SSHUser,
			IdentityFile: conf.TestData.Sshkey,
		})
		if res.Error() != nil {
			return "", fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		}
		return strings.TrimSpace(res.StdOut()), nil
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Equal(equal))
}
